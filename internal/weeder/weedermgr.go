package weeder

import (
	"context"
	"sync"
)

// Manager provides a single point for registering and unregistering weeders
type Manager interface {
	Register(weeder Weeder) bool
	Unregister(key string) bool
	UnregisterAll() bool
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

// close cancels the weeder
func (wr weederRegistration) close() {
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

// NewManager creates a new manager to manager weeder registrations
func NewManager() *weederManager {
	return &weederManager{
		weeders: make(map[string]weederRegistration),
	}
}

// Unregister cancels the weeder registration if one exists using the passed key (see `createKey` function)
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
		if wm.Unregister(key) != true {
			return false
		}
	}
	return true
}

// createKey creates a key to uniquely identify a weeder
func createKey(w Weeder) string {
	return w.namespace + "/" + w.endpoints.Name
}

func (wm *weederManager) GetWeederRegistration(key string) (weederRegistration, bool) {
	wr, ok := wm.weeders[key]
	return wr, ok
}
