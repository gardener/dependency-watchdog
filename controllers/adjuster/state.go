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
	// freshMachineInfos is an expiring map of machine names to machineBasicInfo
	freshMachineInfos *cache.Expiring
	// stats is an expiring map of [adjustapi.ProvisionKey] to statData
	stats *cache.Expiring
}

type machineBasicInfo struct {
	namespacedName types.NamespacedName
	provisionKey   adjustapi.ProvisionKey
	expiry         time.Duration
	joined         bool
	failed         bool
}

type statData struct {
	expiry                 time.Duration
	createDuration         time.Duration
	joinDuration           time.Duration
	createCount            int32
	joinCount              int32
	failCount              int32
	relatedDeploymentNames sets.Set[types.NamespacedName]
}

func (s statData) String() string {
	return fmt.Sprintf("(createCount=%d,joinCount=%d,failCount=%d,createDuration=%d,joinDuration=%d,relatedDeploymentNames=%s,expiry=%s)",
		s.createCount, s.joinCount, s.failCount, s.createDuration, s.joinDuration, s.relatedDeploymentNames, s.expiry)
}

type statUpdateFunc func(existingStat *statData) (updatedStat statData)
type statGetFunc[T any] func(existingStat *statData) T

func (s *state) isRecorded(nn types.NamespacedName) bool {
	_, ok := s.getMachineBasicInfo(nn)
	return ok
}

func (s *state) isJoinRecorded(nn types.NamespacedName) bool {
	basicInfo, ok := s.getMachineBasicInfo(nn)
	if !ok {
		return false
	}
	return basicInfo.joined
}

func (s *state) isFailRecorded(nn types.NamespacedName) bool {
	basicInfo, ok := s.getMachineBasicInfo(nn)
	if !ok {
		return false
	}
	return basicInfo.failed
}

func (s *state) recordFresh(basicInfo machineBasicInfo, deploymentName string, createDuration time.Duration) statData {
	s.freshMachineInfos.Set(basicInfo.namespacedName, basicInfo, basicInfo.expiry)
	return s.updateStatWithFreshData(basicInfo.provisionKey, statData{
		expiry:                 basicInfo.expiry,
		createCount:            1,
		createDuration:         createDuration,
		relatedDeploymentNames: sets.New(types.NamespacedName{Name: deploymentName, Namespace: basicInfo.namespacedName.Namespace}),
	})
}

func (s *state) recordJoin(nn types.NamespacedName, joinDuration time.Duration) (basicInfo machineBasicInfo, data statData, ok bool) {
	basicInfo, ok = s.getMachineBasicInfo(nn)
	if !ok {
		return
	}
	basicInfo.joined = true
	s.freshMachineInfos.Set(nn, basicInfo, basicInfo.expiry)
	data = s.updateStatWithFreshData(basicInfo.provisionKey, statData{
		joinDuration: joinDuration,
		joinCount:    1,
	})
	return
}

func (s *state) recordFail(nn types.NamespacedName) (basicInfo machineBasicInfo, data statData, ok bool) {
	basicInfo, ok = s.getMachineBasicInfo(nn)
	if !ok {
		return
	}
	basicInfo.failed = true
	s.freshMachineInfos.Set(nn, basicInfo, basicInfo.expiry)
	data = s.updateStatWithFreshData(basicInfo.provisionKey, statData{
		failCount: 1,
	})
	return
}

// updateStatWithFreshData updates the existing statData in the backing stats map with the given statData or inserts the
// given statData using the given statKey. the entry expires after stateData.expiry.
func (s *state) updateStatWithFreshData(key adjustapi.ProvisionKey, data statData) statData {
	return performStatUpdate(s.stats, s.mu, key, func(existingData *statData) (updatedStat statData) {
		if existingData == nil {
			return data
		}
		data.createDuration = max(existingData.createDuration, data.createDuration)
		data.createCount += existingData.createCount
		data.joinDuration = max(existingData.joinDuration, data.joinDuration)
		data.joinCount += existingData.joinCount
		data.failCount += existingData.failCount
		data.relatedDeploymentNames = existingData.relatedDeploymentNames.Union(data.relatedDeploymentNames)
		if data.expiry == 0 {
			data.expiry = existingData.expiry
		}
		return data
	})
}

func (s *state) getMachineDeploymentNames(key adjustapi.ProvisionKey) sets.Set[types.NamespacedName] {
	return performStatGet(s.stats, s.mu, key, func(existingStat *statData) sets.Set[types.NamespacedName] {
		return existingStat.relatedDeploymentNames
	})
}

func (s *state) getMachineBasicInfo(machineNamespacedName types.NamespacedName) (info machineBasicInfo, ok bool) {
	val, ok := s.freshMachineInfos.Get(machineNamespacedName)
	if !ok {
		return
	}
	info = val.(machineBasicInfo)
	return
}

func (s *state) clearDataForMachine(machineName types.NamespacedName) {
	s.freshMachineInfos.Delete(machineName)
}

func (s *state) removeFromRelatedDeployments(provKey adjustapi.ProvisionKey, deploymentNames ...types.NamespacedName) {
	performStatUpdate(s.stats, s.mu, provKey, func(existingStat *statData) (updatedStat statData) {
		existingStat.relatedDeploymentNames.Delete(deploymentNames...)
		return *existingStat
	})
}

func performStatUpdate(stats *cache.Expiring, mu *sync.Mutex, key adjustapi.ProvisionKey, updateFn statUpdateFunc) statData {
	mu.Lock()
	defer mu.Unlock()
	var (
		existingData, updatedData statData
	)
	val, ok := stats.Get(key)
	if !ok {
		updatedData = updateFn(nil)
	} else {
		existingData = val.(statData)
		updatedData = updateFn(&existingData)
	}
	stats.Set(key, updatedData, updatedData.expiry)
	return updatedData
}

func performStatGet[T any](stats *cache.Expiring, mu *sync.Mutex, key adjustapi.ProvisionKey, getFn statGetFunc[T]) T {
	mu.Lock()
	defer mu.Unlock()
	val, ok := stats.Get(key)
	if !ok {
		return getFn(nil)
	}
	return getFn(new(val.(statData)))
}
