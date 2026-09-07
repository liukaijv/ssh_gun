package procman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type runtimeEntry struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Command   string    `json:"command"`
}

type runtimeFile struct {
	Processes map[string]runtimeEntry `json:"processes"`
}

type runtimeStore struct {
	mu   sync.Mutex
	path string
}

func newRuntimeStore(path string) *runtimeStore {
	if path == "" {
		return nil
	}
	return &runtimeStore{path: path}
}

func (s *runtimeStore) loadAll() map[string]runtimeEntry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return map[string]runtimeEntry{}
	}
	var file runtimeFile
	if err := json.Unmarshal(data, &file); err != nil || file.Processes == nil {
		return map[string]runtimeEntry{}
	}
	return file.Processes
}

func (s *runtimeStore) save(id string, entry runtimeEntry) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.readLocked()
	file.Processes[id] = entry
	return s.writeLocked(file)
}

func (s *runtimeStore) remove(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.readLocked()
	if _, ok := file.Processes[id]; !ok {
		return
	}
	delete(file.Processes, id)
	_ = s.writeLocked(file)
}

func (s *runtimeStore) readLocked() runtimeFile {
	file := runtimeFile{Processes: map[string]runtimeEntry{}}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return file
	}
	if err := json.Unmarshal(data, &file); err != nil || file.Processes == nil {
		return runtimeFile{Processes: map[string]runtimeEntry{}}
	}
	return file
}

func (s *runtimeStore) writeLocked(file runtimeFile) error {
	if file.Processes == nil {
		file.Processes = map[string]runtimeEntry{}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
