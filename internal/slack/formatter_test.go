package slack

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/outdoorsea/gt-herald/internal/watcher"
)

func TestFormatEvent_Crash(t *testing.T) {
	event := watcher.Event{
		Timestamp: time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC),
		Type:      "crash",
		Agent:     "vitalitek/polecats/onyx",
		Context:   "Session died unexpectedly",
	}
	payload := FormatEvent(event)
	if len(payload.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(payload.Blocks))
	}
	// Should be valid JSON.
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	// Header should contain the agent and type.
	header := payload.Blocks[0].Text.Text
	if header == "" {
		t.Fatal("header text is empty")
	}
	if !contains(header, "vitalitek/polecats/onyx") {
		t.Errorf("header missing agent: %s", header)
	}
	if !contains(header, ":rotating_light:") {
		t.Errorf("crash should use rotating_light emoji: %s", header)
	}
}

func TestFormatEvent_Spawn(t *testing.T) {
	event := watcher.Event{
		Timestamp: time.Now(),
		Type:      "spawn",
		Agent:     "gastown/polecats/Toast",
		Context:   "gt-h8x",
	}
	payload := FormatEvent(event)
	header := payload.Blocks[0].Text.Text
	if !contains(header, ":hatching_chick:") {
		t.Errorf("spawn should use hatching_chick emoji: %s", header)
	}
}

func TestFormatEvent_NoContext(t *testing.T) {
	event := watcher.Event{
		Timestamp: time.Now(),
		Type:      "done",
		Agent:     "gastown/crew/beercan",
	}
	payload := FormatEvent(event)
	// Should have header + timestamp, no context block.
	if len(payload.Blocks) != 2 {
		t.Fatalf("expected 2 blocks (no context), got %d", len(payload.Blocks))
	}
}

func TestFormatBatch(t *testing.T) {
	events := []watcher.Event{
		{Type: "spawn", Agent: "gastown/polecats/a"},
		{Type: "spawn", Agent: "gastown/polecats/b"},
		{Type: "done", Agent: "gastown/polecats/c"},
	}
	payload := FormatBatch(events, "gastown")
	if len(payload.Blocks) == 0 {
		t.Fatal("batch payload has no blocks")
	}
	header := payload.Blocks[0].Text.Text
	if !contains(header, "3 events") {
		t.Errorf("batch header should mention 3 events: %s", header)
	}
	if !contains(header, "gastown") {
		t.Errorf("batch header should mention rig: %s", header)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
