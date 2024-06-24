// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"github.com/gardener/dependency-watchdog/internal/prober/types"
	"sync"
)

// NewManager creates a new manager to manage probers.
func NewManager() types.Manager {
	return &manager{
		probers: make(map[string]types.Prober),
	}
}

type manager struct {
	sync.Mutex
	probers map[string]types.Prober
}

func (pm *manager) Unregister(key string) bool {
	pm.Lock()
	defer pm.Unlock()
	if probe, ok := pm.probers[key]; ok {
		delete(pm.probers, key)
		probe.Close()
		return true
	}
	return false
}

func (pm *manager) Register(prober types.Prober) bool {
	pm.Lock()
	defer pm.Unlock()
	key := createKey(prober)
	if _, ok := pm.probers[key]; !ok {
		pm.probers[key] = prober
		return true
	}
	return false
}

func (pm *manager) GetProber(key string) (types.Prober, bool) {
	prober, ok := pm.probers[key]
	return prober, ok
}

func (pm *manager) GetAllProbers() []types.Prober {
	probers := make([]types.Prober, 0, len(pm.probers))
	for _, p := range pm.probers {
		probers = append(probers, p)
	}
	return probers
}

func createKey(prober types.Prober) string {
	return prober.Namespace // check if this would be sufficient
}
