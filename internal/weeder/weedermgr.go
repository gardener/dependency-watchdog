package weeder

import "sync"

type WeederManager interface {
	Register(weeder Weeder) bool
	Unregister(key string) bool
}

func NewWeederManager() *weederManager {
	return &weederManager{
		weeders: make(map[string]Weeder),
	}
}

type weederManager struct {
	sync.Mutex
	weeders map[string]Weeder
}

//Register registers the new weeder
func (wm *weederManager) Register(weeder Weeder) bool {
	//checks if there is an existing registration. If yes then it will get the Weeder and check isClosed and if not then call Close()
	wm.Lock()
	defer wm.Unlock()
	key := createKey(weeder)
	if w, ok := wm.weeders[key]; ok {
		if !w.isClosed() {
			weeder.Close()
		}
	}
	wm.weeders[key] = weeder
	return true
}

func (wm *weederManager) Unregister(key string) bool {
	wm.Lock()
	defer wm.Unlock()
	if w, ok := wm.weeders[key]; ok {
		delete(wm.weeders, key)
		w.Close()
		return true
	}
	return false
}

func createKey(w Weeder) string {
	return w.endpoints.Name + w.namespace
}
