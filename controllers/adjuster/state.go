package adjuster

import (
	"fmt"
	"sync"
	"time"

	adjustapi "github.com/gardener/dependency-watchdog/api/adjuster"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
)

type state struct {
	mu *sync.Mutex
	// machineTrackInfos is an expiring map of machine namespaced name to machineTrackInfo
	machineTrackInfos *cache.Expiring
	// deploymentStats is an expiring map of MachineDeployment namespaced name to machineDeploymentStat
	deploymentStats *cache.Expiring
	// provisionKeyEntries is an expiring map of [adjustapi.ProvisionKey] to provisionKeyEntry
	provisionKeyEntries *cache.Expiring
}

// machineTrackInfo represents minimal tracking information about a Machine object - its [adjustapi.ProvisionKey]
// and whether it has joined the cluster or considered failed.
type machineTrackInfo struct {
	provisionKey   adjustapi.ProvisionKey
	namespacedName types.NamespacedName
	ttl            time.Duration
	joined         bool
	failed         bool
}

func (t machineTrackInfo) String() string {
	return fmt.Sprintf("(provisionKey=%s,namespacedName=%s,joined=%t,failed=%t,ttl=%s)",
		t.provisionKey, t.namespacedName, t.joined, t.failed, t.ttl)
}

// machineDeploymentStats stores aggregated observations for a MachineDeployment within a ttl.
type machineDeploymentStat struct {
	ttl            time.Duration
	createDuration time.Duration // createDuration maximum
	joinDuration   time.Duration // join duration maximum
	joinCount      uint32        // recent join counts after last failure
	failCount      uint32        // recent failure counts after last join
}

func (s machineDeploymentStat) String() string {
	return fmt.Sprintf("(joinCount=%d,failCount=%d,createDuration=%s,joinDuration=%s,ttl=%s)",
		s.joinCount, s.failCount, s.createDuration, s.joinDuration, s.ttl)
}

// provisionKeyTrackInfo stores information associated with a [adjustapi.ProvisionKey]
type provisionKeyEntry struct {
	ttl             time.Duration
	deploymentNames sets.Set[types.NamespacedName]
}

type statUpdateFunc func(existingStat *machineDeploymentStat) (updatedStat machineDeploymentStat)
type statGetFunc[T any] func(existingStat *machineDeploymentStat) T

func (s *state) isRecorded(nn types.NamespacedName) bool {
	_, ok := s.getMachineTrackInfo(nn)
	return ok
}

func (s *state) isJoinRecorded(nn types.NamespacedName) bool {
	trackInfo, ok := s.getMachineTrackInfo(nn)
	if ok {
		return trackInfo.joined
	}
	return false
}

func (s *state) isFailRecorded(nn types.NamespacedName) bool {
	trackInfo, ok := s.getMachineTrackInfo(nn)
	if ok {
		return trackInfo.failed
	}
	return false
}

func (s *state) trackMachineCreate(trackInfo machineTrackInfo, deploymentName string, createDuration time.Duration) machineDeploymentStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.machineTrackInfos.Set(trackInfo.namespacedName, trackInfo, trackInfo.ttl)
	mcdNsName := types.NamespacedName{Namespace: trackInfo.namespacedName.Namespace, Name: deploymentName}
	deploymentStat := s.updateMachineDeploymentStat(mcdNsName, machineDeploymentStat{
		createDuration: createDuration,
		ttl:            trackInfo.ttl})
	s.recordProvisionEntry(trackInfo.provisionKey, mcdNsName, deploymentStat.ttl)
	return deploymentStat
}

func (s *state) trackMachineJoin(machineNsName types.NamespacedName, mcdNsName types.NamespacedName, joinDuration time.Duration) (trackInfo machineTrackInfo, deploymentStat machineDeploymentStat, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trackInfo, ok = s.getMachineTrackInfo(machineNsName)
	if !ok {
		return
	}
	trackInfo.joined = true
	s.machineTrackInfos.Set(machineNsName, trackInfo, trackInfo.ttl)
	deploymentStat = s.updateMachineDeploymentStat(mcdNsName, machineDeploymentStat{
		joinDuration: joinDuration,
		joinCount:    1,
	})
	s.recordProvisionEntry(trackInfo.provisionKey, mcdNsName, deploymentStat.ttl)
	return
}

func (s *state) trackMachineFail(nn types.NamespacedName, deploymentName string) (trackInfo machineTrackInfo, deploymentStat machineDeploymentStat, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trackInfo, ok = s.getMachineTrackInfo(nn)
	if !ok {
		return
	}
	trackInfo.failed = true
	s.machineTrackInfos.Set(nn, trackInfo, trackInfo.ttl)
	mcdNsName := types.NamespacedName{Namespace: trackInfo.namespacedName.Namespace, Name: deploymentName}
	deploymentStat = s.updateMachineDeploymentStat(mcdNsName, machineDeploymentStat{
		failCount: 1,
	})
	s.recordProvisionEntry(trackInfo.provisionKey, mcdNsName, deploymentStat.ttl)
	return
}

// updateMachineDeploymentStat updates the existing machineDeploymentStat in the backing deploymentStats map with the given
// data or inserts the given data. The entry expires after data.ttl. The key for the deploymentStatus map is the
// [types.NamespacedName] of the MachineDeployment.
func (s *state) updateMachineDeploymentStat(mcdNsName types.NamespacedName, data machineDeploymentStat) machineDeploymentStat {
	var (
		existingData, updatedData machineDeploymentStat
	)
	val, ok := s.deploymentStats.Get(mcdNsName)
	if !ok {
		updatedData = data
	} else {
		existingData = val.(machineDeploymentStat)
		updatedData = combineDeploymentStat(existingData, data)
	}
	s.deploymentStats.Set(mcdNsName, updatedData, updatedData.ttl)
	return updatedData
}

func (s *state) getMachineDeploymentNamespacedNames(key adjustapi.ProvisionKey) sets.Set[types.NamespacedName] {
	entry, ok := s.getProvisionKeyEntry(key)
	if !ok {
		return nil
	}
	return entry.deploymentNames
}

func (s *state) getMachineTrackInfo(machineNamespacedName types.NamespacedName) (info machineTrackInfo, ok bool) {
	val, ok := s.machineTrackInfos.Get(machineNamespacedName)
	if !ok {
		return
	}
	info = val.(machineTrackInfo)
	return
}

func (s *state) clearDataForMachine(machineName types.NamespacedName) {
	s.machineTrackInfos.Delete(machineName)
}

func (s *state) recordProvisionEntry(key adjustapi.ProvisionKey, mcdNsName types.NamespacedName, ttl time.Duration) {
	var nameSet sets.Set[types.NamespacedName]
	entry, ok := s.getProvisionKeyEntry(key)
	if ok {
		nameSet = entry.deploymentNames
	} else {
		nameSet = sets.New[types.NamespacedName]()
	}
	nameSet.Insert(mcdNsName)
	entry.ttl = ttl
	entry.deploymentNames = nameSet
	s.provisionKeyEntries.Set(key, entry, ttl)
}

func (s *state) performStatUpdate(mcdNsName types.NamespacedName, updateFn statUpdateFunc) machineDeploymentStat {
	var (
		existingData, updatedData machineDeploymentStat
	)
	val, ok := s.deploymentStats.Get(mcdNsName)
	if !ok {
		updatedData = updateFn(nil)
	} else {
		existingData = val.(machineDeploymentStat)
		updatedData = updateFn(&existingData)
	}
	s.deploymentStats.Set(mcdNsName, updatedData, updatedData.ttl)
	return updatedData
}

func (s *state) clearStateForDeployments(key adjustapi.ProvisionKey, notFoundNames sets.Set[types.NamespacedName]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, dn := range notFoundNames.UnsortedList() {
		s.deploymentStats.Delete(dn)
		entry, ok := s.getProvisionKeyEntry(key)
		if !ok {
			continue
		}
		if !entry.deploymentNames.Has(dn) {
			continue
		}
		entry.deploymentNames.Delete(dn)
		s.provisionKeyEntries.Set(key, entry, entry.ttl)
	}
}

func (s *state) getProvisionKeyEntry(provisionKey adjustapi.ProvisionKey) (entry provisionKeyEntry, ok bool) {
	val, ok := s.provisionKeyEntries.Get(provisionKey)
	if !ok {
		return
	}
	entry = val.(provisionKeyEntry)
	return
}
