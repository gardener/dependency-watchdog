// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"sync"
)

// Manager is the convenience interface to manage lifecycle of probers.
type Manager interface {
	// Register registers the given prober with the manager. It should return false if prober is already registered.
	Register(prober Prober) bool
	// Unregister closes the prober and removes it from the manager. It should return false if prober is not registered with the manager.
	Unregister(key string) bool
	// GetProber uses the given key to get a registered prober from the manager. It returns false if prober is not found.
	GetProber(key string) (Prober, bool)
	// GetAllProbers returns a slice of all the probers registered with the manager.
	GetAllProbers() []Prober
}

// NewManager creates a new manager to manage probers.
func NewManager() Manager {
	return &manager{
		probers: make(map[string]Prober),
	}
}

type manager struct {
	sync.Mutex
	probers map[string]Prober
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

func (pm *manager) Register(prober Prober) bool {
	pm.Lock()
	defer pm.Unlock()
	key := createKey(prober)
	if _, ok := pm.probers[key]; !ok {
		pm.probers[key] = prober
		return true
	}
	return false
}

func (pm *manager) GetProber(key string) (Prober, bool) {
	prober, ok := pm.probers[key]
	return prober, ok
}

func (pm *manager) GetAllProbers() []Prober {
	probers := make([]Prober, 0, len(pm.probers))
	for _, p := range pm.probers {
		probers = append(probers, p)
	}
	return probers
}

func createKey(prober Prober) string {
	return prober.namespace // check if this would be sufficient
}
