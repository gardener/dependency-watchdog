package prober

import "sync"

type Manager interface {
	Register(prober *Prober)
	Unregister(key string)
	GetProber(key string) (*Prober, bool)
}

func NewManager() Manager {
	return &manager{
		probers: make(map[string]*Prober),
	}
}

type manager struct {
	sync.Mutex
	probers map[string]*Prober
}

func (pm *manager) Unregister(key string) {
	pm.Lock()
	defer pm.Unlock()
	if probe, ok := pm.probers[key]; ok {
		delete(pm.probers, key)
		probe.Close()
	}
}

func (pm *manager) Register(prober *Prober) {
	pm.Lock()
	defer pm.Unlock()
	key := createKey(prober)
	if _, ok := pm.probers[key]; !ok {
		pm.probers[key] = prober
	}
}

func (pm *manager) GetProber(key string) (*Prober, bool) {
	prober, ok := pm.probers[key]
	return prober, ok
}

func createKey(prober *Prober) string {
	return prober.Namespace // check if this would be sufficient
}
