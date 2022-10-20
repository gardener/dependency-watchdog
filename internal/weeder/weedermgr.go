// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	UnregisterAll() bool
	// GetWeederRegistration returns a weederRegistration which will give access to the context and the cancelFn to the caller.
	GetWeederRegistration(key string) (weederRegistration, bool)
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

// isClosed checks if the weeder has been closed
func (wr weederRegistration) isClosed() bool {
	select {
	case <-wr.ctx.Done():
		return true
	default:
		return false
	}
}

// Register registers the new weeder. If the weeder with the same key (see `createKey` function) exists
// then it will close the registration (if not already closed) which cancels the weeder.
// It will then create a new weeder registration which will replace the existing weeder registration.
func (wm *weederManager) Register(weeder Weeder) bool {
	wm.Lock()
	defer wm.Unlock()
	key := createKey(weeder)
	if wr, exists := wm.weeders[key]; exists {
		if !wr.isClosed() {
			wr.close()
		}
	}
	wm.weeders[key] = weederRegistration{
		ctx:      weeder.ctx,
		cancelFn: weeder.cancelFn,
	}
	return true
}

func NewManager() *weederManager {
	return &weederManager{
		weeders: make(map[string]weederRegistration),
	}
}

func (wm *weederManager) Unregister(key string) bool {
	wm.Lock()
	defer wm.Unlock()
	if wr, ok := wm.weeders[key]; ok {
		delete(wm.weeders, key)
		wr.close()
		return true
	}
	return false
}

func (wm *weederManager) UnregisterAll() bool {
	for key := range wm.weeders {
		if !wm.Unregister(key) {
			return false
		}
	}
	return true
}

func (wm *weederManager) GetWeederRegistration(key string) (weederRegistration, bool) {
	wr, ok := wm.weeders[key]
	return wr, ok
}

// createKey creates a key to uniquely identify a weeder
func createKey(w Weeder) string {
	return w.namespace + "/" + w.endpoints.Name
}

// close cancels the weeder
func (wr weederRegistration) close() {
	wr.cancelFn()
}
