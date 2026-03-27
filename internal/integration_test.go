package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/outdoorsea/gt-herald/internal/router"
	"github.com/outdoorsea/gt-herald/internal/slack"
	"github.com/outdoorsea/gt-herald/internal/state"
	"github.com/outdoorsea/gt-herald/internal/watcher"
	"github.com/outdoorsea/gt-herald/internal/config"
)

// TestEndToEnd writes townlog lines, runs the watcher+router+sink pipeline,
// and verifies Slack webhook POSTs are received with correct formatting.
func TestEndToEnd(t *testing.T) {
	// Set up temp directory with townlog.
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(logDir, "town.log")

	// Write initial townlog content.
	initial := "2026-03-26 12:00:00 [spawn] gastown/polecats/Toast gt-h8x\n" +
		"2026-03-26 12:00:01 [nudge] gastown/polecats/Toast check your mail\n" +
		"2026-03-26 12:00:02 [crash] vitalitek/polecats/onyx Session died\n"
	if err := os.WriteFile(logFile, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up mock Slack webhook.
	var mu sync.Mutex
	var received []slack.Payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload slack.Payload
		json.Unmarshal(body, &payload)
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	// Config.
	cfg := &config.Config{
		Slack: config.SlackConfig{WebhookURLEnv: "TEST_WEBHOOK"},
		GasTown: config.GasTownConfig{Root: tmpDir},
		Channels: config.ChannelConfig{
			Alerts:  "#gt-alerts",
			Default: "#gt-ops",
			Rigs: map[string]string{
				"gastown":   "#gt-gastown",
				"vitalitek": "#gt-vitalitek",
			},
		},
		Filters: config.FilterConfig{
			Townlog: config.TownlogFilter{
				Include: []string{"spawn", "done", "crash", "session_death", "handoff"},
			},
		},
		RateLimit: config.RateLimitConfig{PerChannel: 10, Burst: 5, BatchWindow: "1s"},
		StateFile: filepath.Join(tmpDir, "state.json"),
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sink := slack.NewSink(server.URL, cfg.RateLimit.PerChannel, cfg.RateLimit.BatchWindowDuration(), log)
	rtr := router.New(cfg)
	tw := watcher.NewTownlogWatcher(logFile, 0, log)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start watcher.
	go tw.Start(ctx)

	// Process events.
	deadline := time.After(3 * time.Second)
	processed := 0
loop:
	for {
		select {
		case event, ok := <-tw.Events():
			if !ok {
				break loop
			}
			if routed, include := rtr.Route(event); include {
				sink.Send(ctx, routed.Channel, event)
				processed++
			}
			// We expect 2 posts: spawn + crash (nudge is suppressed).
			if processed >= 2 {
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	cancel()

	// Verify.
	mu.Lock()
	defer mu.Unlock()

	if len(received) != 2 {
		t.Fatalf("expected 2 webhook POSTs, got %d", len(received))
	}

	// First should be spawn.
	if received[0].Blocks[0].Text == nil || received[0].Blocks[0].Text.Text == "" {
		t.Error("first payload has no header text")
	}

	// Second should be crash.
	if received[1].Blocks[0].Text == nil || received[1].Blocks[0].Text.Text == "" {
		t.Error("second payload has no header text")
	}

	// Verify state persistence.
	st := &state.State{TownlogOffset: tw.Offset()}
	if err := st.Save(cfg.StateFile); err != nil {
		t.Fatalf("save state: %v", err)
	}
	loaded, err := state.Load(cfg.StateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loaded.TownlogOffset != st.TownlogOffset {
		t.Errorf("state offset = %d, want %d", loaded.TownlogOffset, st.TownlogOffset)
	}
	if loaded.TownlogOffset == 0 {
		t.Error("state offset should be > 0 after reading lines")
	}
}

// TestStateRecovery verifies that the watcher resumes from saved offset.
func TestStateRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "town.log")

	// Write 3 lines.
	lines := "2026-03-26 12:00:00 [spawn] gastown/polecats/a first\n" +
		"2026-03-26 12:00:01 [spawn] gastown/polecats/b second\n" +
		"2026-03-26 12:00:02 [spawn] gastown/polecats/c third\n"
	os.WriteFile(logFile, []byte(lines), 0644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// First run: read all 3 lines.
	tw1 := watcher.NewTownlogWatcher(logFile, 0, log)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	go tw1.Start(ctx1)
	count := 0
	for count < 3 {
		select {
		case <-tw1.Events():
			count++
		case <-ctx1.Done():
			t.Fatalf("timeout waiting for events, got %d", count)
		}
	}
	cancel1()
	offset := tw1.Offset()

	// Append 1 more line.
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("2026-03-26 12:00:03 [done] gastown/polecats/d fourth\n")
	f.Close()

	// Second run: resume from saved offset, should only see the new line.
	tw2 := watcher.NewTownlogWatcher(logFile, offset, log)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	go tw2.Start(ctx2)
	select {
	case event := <-tw2.Events():
		if event.Agent != "gastown/polecats/d" {
			t.Errorf("expected agent 'd', got %q", event.Agent)
		}
		if event.Type != "done" {
			t.Errorf("expected type 'done', got %q", event.Type)
		}
	case <-ctx2.Done():
		t.Fatal("timeout waiting for resumed event")
	}
	cancel2()
}

// TestRateLimiting verifies that events are batched when rate limit is exceeded.
func TestRateLimiting(t *testing.T) {
	var mu sync.Mutex
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		postCount++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer server.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sink := slack.NewSink(server.URL, 2, 100*time.Millisecond, log) // max 2/min

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		sink.Send(ctx, "#test", watcher.Event{
			Type:  "spawn",
			Agent: "gastown/polecats/test",
		})
	}

	// Only 2 should have been posted (3 buffered).
	mu.Lock()
	immediate := postCount
	mu.Unlock()
	if immediate != 2 {
		t.Errorf("expected 2 immediate posts, got %d", immediate)
	}

	// Flush should send batch summary.
	sink.FlushBatches(ctx)
	time.Sleep(100 * time.Millisecond) // let HTTP complete

	mu.Lock()
	total := postCount
	mu.Unlock()
	if total != 3 { // 2 immediate + 1 batch summary
		t.Errorf("expected 3 total posts after flush, got %d", total)
	}
}
