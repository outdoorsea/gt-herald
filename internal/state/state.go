// Package state provides cursor persistence for restart recovery.
package state

import (
	"encoding/json"
	"os"
	"time"
)

// State tracks cursor positions for gt-herald event sources.
type State struct {
	TownlogOffset int64              `json:"townlog_offset"`
	BeadsCursors  map[string]string  `json:"beads_cursors,omitempty"` // rig → last updated_at (RFC3339)
	UpdatedAt     time.Time          `json:"updated_at"`
}

// Load reads state from a JSON file. Returns zero state if file doesn't exist.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return &State{}, nil // corrupt state — start fresh
	}
	return &s, nil
}

// Save writes state to a JSON file atomically (write tmp + rename).
func (s *State) Save(path string) error {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
