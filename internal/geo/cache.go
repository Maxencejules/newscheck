package geo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Cache struct {
	mu      sync.RWMutex
	inMem   map[string]CountryInfo // key: normalized query name
	path    string
	loaded  bool
	enabled bool
}

func NewCache(appName string) *Cache {
	dir, err := os.UserConfigDir()
	enabled := err == nil
	var p string
	if enabled {
		p = filepath.Join(dir, appName, "country_cache.json")
	}
	return &Cache{
		inMem:   map[string]CountryInfo{},
		path:    p,
		enabled: enabled,
	}
}

func (c *Cache) Get(key string) (CountryInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.inMem[key]
	return v, ok
}

func (c *Cache) Put(key string, v CountryInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inMem[key] = v
	if !c.enabled {
		return nil
	}
	return c.saveLocked()
}

func (c *Cache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return nil
	}
	c.loaded = true

	if !c.enabled {
		return nil
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var m map[string]CountryInfo
	if err := json.Unmarshal(data, &m); err != nil {
		// If cache is corrupted, ignore it rather than failing the app.
		return nil
	}

	for k, v := range m {
		c.inMem[k] = v
	}
	return nil
}

func (c *Cache) saveLocked() error {
	if !c.enabled {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c.inMem, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
