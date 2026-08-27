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
	// provisionKeyStats is an expiring map of [adjustapi.MachineProvisionKey] to machineProvisionKeyStat
	provisionKeyStats *cache.Expiring
}

// machineTrackInfo represents minimal tracking information about a Machine object - its [adjustapi.MachineProvisionKey]
// and whether it has joined the cluster or considered failed. Information is only preserved for a ttl.
type machineTrackInfo struct {
	provisionKey   adjustapi.MachineProvisionKey
	namespacedName types.NamespacedName
	ttl            time.Duration
	joined         bool
	failed         bool
}

func (t machineTrackInfo) String() string {
	return fmt.Sprintf("(provisionKey=%s,namespacedName=%s,joined=%t,failed=%t,ttl=%s)",
		t.provisionKey, t.namespacedName, t.joined, t.failed, t.ttl)
}

// machineDeploymentStats stores some observations for a MachineDeployment - the machine join streak count and machine fail streak count.
// The stat is associated with a ttl that is refreshed whenever a machine belonging to the MachineDeployment is tracked.
type machineDeploymentStat struct {
	ttl             time.Duration
	maxJoinDuration time.Duration // max machine join duration maximum
	joinStreakCount uint32        // recent join counts after last failure
	failStreakCount uint32        // recent failure counts after last join
}

func (s machineDeploymentStat) String() string {
	return fmt.Sprintf("(joinStreakCount=%d,failStreakCount=%d,maxJoinDuration=%s,ttl=%s)",
		s.joinStreakCount, s.failStreakCount, s.maxJoinDuration, s.ttl)
}

// machineProvisionKeyStat stores observations associated with a [adjustapi.MachineProvisionKey]
type machineProvisionKeyStat struct {
	ttl             time.Duration
	joinCount       uint32
	failCount       uint32
	deploymentNames sets.Set[types.NamespacedName]
}

func (s machineProvisionKeyStat) String() string {
	return fmt.Sprintf("(joinCount=%d,failCount=%d,#deploymentNames=%d,ttl=%s)",
		s.joinCount, s.failCount, len(s.deploymentNames), s.ttl)
}

// HasBreached confirms whether the failCount > thresholdMin and the fraction failCount/failCount+jointCount has breached
// the given thresholdFraction.
func (s machineProvisionKeyStat) HasBreached(thresholdMin uint32, thresholdFraction float64) bool {
	if s.failCount < thresholdMin {
		return false
	}
	total := s.failCount + s.joinCount
	if total == 0 {
		return false
	}
	return float64(s.failCount)/float64(total) >= thresholdFraction
}

type deploymentStatUpdateFunc func(existingStat *machineDeploymentStat) (updatedStat machineDeploymentStat)

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

func (s *state) recordMachineJoin(machineNsName types.NamespacedName, deploymentName string, provisionKey adjustapi.MachineProvisionKey, joinDuration time.Duration, ttl time.Duration) (machineDeploymentStat, machineProvisionKeyStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trackInfo := machineTrackInfo{
		provisionKey:   provisionKey,
		namespacedName: machineNsName,
		ttl:            ttl,
		joined:         true,
	}
	s.machineTrackInfos.Set(machineNsName, trackInfo, trackInfo.ttl)
	mcdNsName := types.NamespacedName{Namespace: trackInfo.namespacedName.Namespace, Name: deploymentName}
	deploymentStat := s.combineUpdateMachineDeploymentStat(mcdNsName, machineDeploymentStat{
		maxJoinDuration: joinDuration,
		joinStreakCount: 1,
		ttl:             ttl,
	})
	provisionKeyStat := s.updateProvisionKeyStat(provisionKey, mcdNsName, 1, 0, deploymentStat.ttl)
	return deploymentStat, provisionKeyStat
}

func (s *state) recordMachineFail(machineNsName types.NamespacedName, deploymentName string, provisionKey adjustapi.MachineProvisionKey, ttl time.Duration) (machineDeploymentStat, machineProvisionKeyStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trackInfo := machineTrackInfo{
		provisionKey:   provisionKey,
		namespacedName: machineNsName,
		ttl:            ttl,
		failed:         true,
	}
	s.machineTrackInfos.Set(machineNsName, trackInfo, trackInfo.ttl)
	mcdNsName := types.NamespacedName{Namespace: trackInfo.namespacedName.Namespace, Name: deploymentName}
	deploymentStat := s.combineUpdateMachineDeploymentStat(mcdNsName, machineDeploymentStat{
		failStreakCount: 1,
		ttl:             ttl,
	})
	provisionKeyStat := s.updateProvisionKeyStat(trackInfo.provisionKey, mcdNsName, 0, 1, deploymentStat.ttl)
	return deploymentStat, provisionKeyStat
}

// updateProvisionKeyStat inserts or updates the machineProvisionKeyStat associated with the given [adjustapi.MachineProvisionKey],
// incrementing the machineProvisionKeyStat's machine join count and fail count with the given joinCount and failCount for the given machine deployment
// and also remembering the machine deployment name.
func (s *state) updateProvisionKeyStat(provisionKey adjustapi.MachineProvisionKey, mcdNsName types.NamespacedName, joinCount, failCount uint32, ttl time.Duration) machineProvisionKeyStat {
	var nameSet sets.Set[types.NamespacedName]
	stat, ok := s.getMachineProvisionKeyStat(provisionKey)
	if ok {
		nameSet = stat.deploymentNames
	} else {
		nameSet = sets.New[types.NamespacedName]()
	}
	nameSet.Insert(mcdNsName)
	stat.ttl = max(stat.ttl, ttl)
	stat.deploymentNames = nameSet
	stat.joinCount += joinCount
	stat.failCount += failCount
	s.provisionKeyStats.Set(provisionKey, stat, ttl)
	return stat
}

// combineUpdateMachineDeploymentStat updates the existing machineDeploymentStat in the backing deploymentStats map by
// combining with the given data or inserts the given data. The entry expires after data.ttl. The key for the
// deploymentStatus map is the [types.NamespacedName] of the MachineDeployment.
func (s *state) combineUpdateMachineDeploymentStat(mcdNsName types.NamespacedName, data machineDeploymentStat) machineDeploymentStat {
	var (
		existingStat, updatedStat machineDeploymentStat
	)
	val, ok := s.deploymentStats.Get(mcdNsName)
	if !ok {
		updatedStat = data
	} else {
		existingStat = val.(machineDeploymentStat)
		updatedStat = combineDeploymentStat(existingStat, data)
	}
	s.deploymentStats.Set(mcdNsName, updatedStat, updatedStat.ttl)
	return updatedStat
}

func (s *state) getMachineDeploymentNamespacedNames(key adjustapi.MachineProvisionKey) sets.Set[types.NamespacedName] {
	entry, ok := s.getMachineProvisionKeyStat(key)
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

func (s *state) untrackMachine(machineName types.NamespacedName) {
	s.machineTrackInfos.Delete(machineName)
}

func (s *state) clearDeploymentStats(key adjustapi.MachineProvisionKey, notFoundNames sets.Set[types.NamespacedName]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, dn := range notFoundNames.UnsortedList() {
		s.deploymentStats.Delete(dn)
		entry, ok := s.getMachineProvisionKeyStat(key)
		if !ok {
			continue
		}
		if !entry.deploymentNames.Has(dn) {
			continue
		}
		entry.deploymentNames.Delete(dn)
		s.provisionKeyStats.Set(key, entry, entry.ttl)
	}
}

func (s *state) getMachineDeploymentStat(mcdNsName types.NamespacedName) (stat machineDeploymentStat, ok bool) {
	val, ok := s.deploymentStats.Get(mcdNsName)
	if !ok {
		return
	}
	stat = val.(machineDeploymentStat)
	return
}

func (s *state) getMachineProvisionKeyStat(provisionKey adjustapi.MachineProvisionKey) (stat machineProvisionKeyStat, ok bool) {
	val, ok := s.provisionKeyStats.Get(provisionKey)
	if !ok {
		return
	}
	stat = val.(machineProvisionKeyStat)
	return
}
