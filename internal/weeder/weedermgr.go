package weeder

import (
	"context"
	"sync"
)

type Manager interface {
	Register(weeder Weeder) bool
	Unregister(key string) bool
}

type weederManager struct {
	sync.Mutex
	weeders map[string]weederRegistration
}

type weederRegistration struct {
	ctx      context.Context
	cancelFn context.CancelFunc
}

func (wr weederRegistration) isClosed() bool {
	select {
	case <-wr.ctx.Done():
		return true
	default:
		return false
	}
}

func (wr weederRegistration) close() {
	wr.cancelFn()
}

//Register registers the new weeder
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

func NewWeederManager() *weederManager {
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

func createKey(w Weeder) string {
	return w.endpoints.Name + w.namespace
}
