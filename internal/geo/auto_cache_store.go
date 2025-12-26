package geo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AutoCacheStore struct {
	path string
	mu   sync.Mutex
	data map[string]DatasetEntry // canonical name -> entry
}

func NewAutoCacheStore(path string) (*AutoCacheStore, error) {
	s := &AutoCacheStore{
		path: filepath.Clean(path),
		data: map[string]DatasetEntry{},
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AutoCacheStore) Get(name string) (DatasetEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[name]
	return e, ok
}

func (s *AutoCacheStore) Upsert(name string, entry DatasetEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" || entry.ISO2 == "" || len(entry.Languages) == 0 {
		return nil
	}

	s.data[name] = entry

	tmp := s.path + ".tmp"
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
