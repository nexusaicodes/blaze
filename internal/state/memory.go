package state

import (
	"sync"
	"time"
)

// memoryStore implements IStateStore without touching the file system. Used
// by --no-state and by tests.
type memoryStore struct {
	mu    sync.Mutex
	state State
}

func newMemoryStore() *memoryStore {
	return &memoryStore{state: emptyState()}
}

// Open resets the store to an empty state; nothing is persisted between
// targets.
func (s *memoryStore) Open(repoKey, targetKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = emptyState()
	return nil
}

// Snapshot returns a deep copy of the current state.
func (s *memoryStore) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.state)
}

// SetReviewed adds or removes a mark.
func (s *memoryStore) SetReviewed(contentHash string, reviewed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reviewed {
		s.state.Reviewed[contentHash] = Mark{ReviewedAt: time.Now().UTC()}
	} else {
		delete(s.state.Reviewed, contentHash)
	}
	return nil
}

// SetNote sets a note. An empty note deletes the entry.
func (s *memoryStore) SetNote(itemKey, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if note == "" {
		delete(s.state.Notes, itemKey)
	} else {
		s.state.Notes[itemKey] = note
	}
	return nil
}
