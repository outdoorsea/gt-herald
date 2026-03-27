package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNonExistent(t *testing.T) {
	s, err := Load("/nonexistent/path/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if s.TownlogOffset != 0 {
		t.Errorf("expected zero offset, got %d", s.TownlogOffset)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := &State{TownlogOffset: 42}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TownlogOffset != 42 {
		t.Errorf("offset = %d, want 42", loaded.TownlogOffset)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte("not json"), 0644)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Should return zero state on corrupt file.
	if s.TownlogOffset != 0 {
		t.Errorf("expected zero offset for corrupt state, got %d", s.TownlogOffset)
	}
}
