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
	key:=createKey(weeder Weeder)
	if _,ok:=wm.weeders[key];ok{

	}
}

func createKey(w Weeder) string{
	return w.endpoints.Name+w.namespace
}