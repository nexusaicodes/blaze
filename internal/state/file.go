package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore persists state as JSON under
// <baseDir>/blaze/state/<repoKey>/<targetKey>.json.
type FileStore struct {
	baseDir string // set by New("file") from os.UserConfigDir(); tests set it directly

	mu    sync.Mutex
	path  string
	state State
}

// Open loads the state for one review target, creating it if absent.
func (s *FileStore) Open(repoKey, targetKey string) error {
	dir := filepath.Join(s.baseDir, "blaze", "state", repoKey)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("state: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, targetKey+".json")

	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.state = emptyState()
			return nil
		}
		return fmt.Errorf("state: read %s: %w", path, err)
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		if renameErr := os.Rename(path, path+".corrupt"); renameErr != nil {
			return fmt.Errorf("state: rename corrupt %s: %w", path, renameErr)
		}
		s.state = emptyState()
		return nil
	}
	if st.Reviewed == nil {
		st.Reviewed = map[string]Mark{}
	}
	if st.Notes == nil {
		st.Notes = map[string]string{}
	}
	s.state = st
	return nil
}

// Snapshot returns a deep copy of the current state.
func (s *FileStore) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.state)
}

// SetReviewed adds or removes a mark and persists.
func (s *FileStore) SetReviewed(contentHash string, reviewed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reviewed {
		s.state.Reviewed[contentHash] = Mark{ReviewedAt: time.Now().UTC()}
	} else {
		delete(s.state.Reviewed, contentHash)
	}
	return s.persist()
}

// SetNote sets a note and persists. An empty note deletes the entry.
func (s *FileStore) SetNote(itemKey, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if note == "" {
		delete(s.state.Notes, itemKey)
	} else {
		s.state.Notes[itemKey] = note
	}
	return s.persist()
}

// persist writes the whole state to a temp file and renames it over the
// real path. The caller holds s.mu.
func (s *FileStore) persist() error {
	s.state.Version = 1
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("state: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("state: rename %s: %w", tmp, err)
	}
	return nil
}
