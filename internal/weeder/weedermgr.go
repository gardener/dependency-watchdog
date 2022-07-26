package weeder

import "sync"

type Manager interface {
	Register(weeder Weeder)
	Unregister(key string) bool
}

//TODO implement methods
type manager struct {
	sync.Mutex
	weeders map[string]Weeder
}

func (m *manager) Register(weeder Weeder) {
	//checks if there is an existing registration. If yes then it will get the Weeder and check isClosed and if not then call Close()
	//registers the new weeder
}
