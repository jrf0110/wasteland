package main

import (
	"fmt"
	"sort"

	"github.com/gastownhall/wasteland/internal/federation"
)

type memConfigStore struct {
	configs map[string]federation.Config
}

func newMemConfigStore(cfg *federation.Config) *memConfigStore {
	store := &memConfigStore{configs: map[string]federation.Config{}}
	if cfg != nil {
		store.configs[cfg.Upstream] = *cfg
	}
	return store
}

func (m *memConfigStore) Load(upstream string) (*federation.Config, error) {
	cfg, ok := m.configs[upstream]
	if !ok {
		return nil, federation.ErrNotJoined
	}
	return &cfg, nil
}

func (m *memConfigStore) Save(cfg *federation.Config) error {
	if cfg == nil || cfg.Upstream == "" {
		return fmt.Errorf("config upstream is required")
	}
	m.configs[cfg.Upstream] = *cfg
	return nil
}

func (m *memConfigStore) Delete(upstream string) error {
	if _, ok := m.configs[upstream]; !ok {
		return federation.ErrNotJoined
	}
	delete(m.configs, upstream)
	return nil
}

func (m *memConfigStore) List() ([]string, error) {
	upstreams := make([]string, 0, len(m.configs))
	for upstream := range m.configs {
		upstreams = append(upstreams, upstream)
	}
	sort.Strings(upstreams)
	return upstreams, nil
}
