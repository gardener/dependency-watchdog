// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package weeder

import (
	"context"
	"sync"
)

// Manager provides a single point for registering and unregistering weeders
type Manager interface {
	// Register registers a weeder with the manager. If a weeder with a key identified by `createKey`
	// exists then it will close it and replace it with the new weeder.
	Register(weeder Weeder) bool
	// Unregister checks if there is an existing weeder with the key. If it is found then it will close the weeder
	// and remove it from the manager.
	Unregister(key string) bool
	// UnregisterAll unregisters all weeders from the manager.
	UnregisterAll()
	// GetWeederRegistration returns a weederRegistration which will give access to the context and the cancelFn to the caller.
	GetWeederRegistration(key string) (Registration, bool)
}

// Registration provides a handle to check if a weeder has been closed and to also close the weeder.
type Registration interface {
	// IsClosed return true if a weeder is closed else returns false.
	IsClosed() bool
	// Close closes the weeder.
	Close()
}

type weederManager struct {
	sync.Mutex
	weeders map[string]weederRegistration
}

// weederRegistration captures the handle to manage a weeder
type weederRegistration struct {
	ctx      context.Context
	cancelFn context.CancelFunc
}

func (wr weederRegistration) IsClosed() bool {
	select {
	case <-wr.ctx.Done():
		return true
	default:
		return false
	}
}

func (wr weederRegistration) Close() {
	wr.cancelFn()
}

// Register registers the new weeder. If the weeder with the same key (see `createKey` function) exists
// then it will close the registration (if not already closed) which cancels the weeder.
// It will then create a new weeder registration which will replace the existing weeder registration.
func (wm *weederManager) Register(weeder Weeder) bool {
	wm.Lock()
	defer wm.Unlock()
	key := createKey(weeder)
	if wr, exists := wm.weeders[key]; exists {
		if !wr.IsClosed() {
			wr.Close()
		}
	}
	wm.weeders[key] = weederRegistration{
		ctx:      weeder.ctx,
		cancelFn: weeder.cancelFn,
	}
	return true
}

// NewManager creates a new manager for weeders.
func NewManager() Manager {
	return &weederManager{
		weeders: make(map[string]weederRegistration),
	}
}

func (wm *weederManager) Unregister(key string) bool {
	wm.Lock()
	defer wm.Unlock()
	if wr, ok := wm.weeders[key]; ok {
		delete(wm.weeders, key)
		wr.Close()
		return true
	}
	return false
}

func (wm *weederManager) UnregisterAll() {
	for key := range wm.weeders {
		_ = wm.Unregister(key)
	}
}

func (wm *weederManager) GetWeederRegistration(key string) (Registration, bool) {
	wr, ok := wm.weeders[key]
	return wr, ok
}

// createKey creates a key to uniquely identify a weeder
func createKey(w Weeder) string {
	return w.namespace + "/" + w.endpoints.Name
}
