package utils

import (
	"sync"

	coreV1 "k8s.io/api/core/v1"
)

type ConfigMapCacheEntry struct {
	version string
	config  *coreV1.ConfigMap
}

// This type is thread-safe.
type ConfigMapCache struct {
	mu sync.Mutex
	// the cache of the latest config maps, keyed by config map name. When we read
	// from k8s a ConfgMap of the same name but at different version, the entry
	// will be replaced.
	cache map[string]ConfigMapCacheEntry
}

func NewConfigMapCache() *ConfigMapCache {
	return &ConfigMapCache{sync.Mutex{}, map[string]ConfigMapCacheEntry{}}
}

func (c *ConfigMapCache) Get(configMapName, version string) (cm *coreV1.ConfigMap, exists bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.cache[configMapName]; ok {
		if entry.version == version {
			return entry.config, true
		}
		return nil, false
	} else {
		return nil, false
	}
}

func (c *ConfigMapCache) Put(name, version string, config *coreV1.ConfigMap) {
	c.mu.Lock()
	c.cache[name] = ConfigMapCacheEntry{version, config}
	c.mu.Unlock()
}
