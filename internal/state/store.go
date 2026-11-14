// Package state persists review marks and notes, the only things Blaze
// writes across runs.
package state

import (
	"fmt"
	"os"
	"time"
)

// Mark records that an item's content hash has been reviewed.
type Mark struct {
	ReviewedAt time.Time `json:"reviewedAt"`
}

// State is the full persisted state for one review target.
type State struct {
	Version  int               `json:"version"`  // 1
	Reviewed map[string]Mark   `json:"reviewed"` // key: Item.ContentHash
	Notes    map[string]string `json:"notes"`    // key: Item.Key
}

// IStateStore loads, mutates and snapshots the state for one review target.
type IStateStore interface {
	// Open loads the state for one review target, creating it if absent.
	Open(repoKey, targetKey string) error
	// Snapshot returns a deep copy of the current state.
	Snapshot() State
	// SetReviewed adds or removes a mark and persists.
	SetReviewed(contentHash string, reviewed bool) error
	// SetNote sets a note and persists. An empty note deletes the entry.
	SetNote(itemKey, note string) error
}

// New returns the IStateStore for backend. "file" persists under
// os.UserConfigDir(); "memory" keeps state in memory only.
func New(backend string) (IStateStore, error) {
	switch backend {
	case "file":
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("state: user config dir: %w", err)
		}
		return &FileStore{baseDir: dir}, nil
	case "memory":
		return newMemoryStore(), nil
	default:
		return nil, fmt.Errorf("state: unknown backend %q", backend)
	}
}

// emptyState returns a State with both maps initialized and empty, so it
// always encodes as "{}" rather than "null".
func emptyState() State {
	return State{Version: 1, Reviewed: map[string]Mark{}, Notes: map[string]string{}}
}

// cloneState returns a deep copy of s.
func cloneState(s State) State {
	out := State{
		Version:  s.Version,
		Reviewed: make(map[string]Mark, len(s.Reviewed)),
		Notes:    make(map[string]string, len(s.Notes)),
	}
	for k, v := range s.Reviewed {
		out.Reviewed[k] = v
	}
	for k, v := range s.Notes {
		out.Notes[k] = v
	}
	return out
}
