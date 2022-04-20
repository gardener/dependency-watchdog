package prober

import "sync"

type Manager interface {
	Register(prober Prober) bool
	Unregister(key string) bool
	GetProber(key string) (Prober, bool)
	GetAllProbers() []Prober
}

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
	return prober.GetNamespace() // check if this would be sufficient
}
