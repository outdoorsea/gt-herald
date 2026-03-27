package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/outdoorsea/gt-herald/internal/watcher"
)

// Sink sends formatted events to a Slack webhook with rate limiting.
type Sink struct {
	webhookURL string
	client     *http.Client
	log        *slog.Logger

	mu          sync.Mutex
	counts      map[string]int       // channel → messages sent this window
	batches     map[string][]watcher.Event // channel → buffered events
	maxPerMin   int
	batchWindow time.Duration
}

// NewSink creates a Slack webhook sink.
func NewSink(webhookURL string, maxPerMin int, batchWindow time.Duration, log *slog.Logger) *Sink {
	return &Sink{
		webhookURL:  webhookURL,
		client:      &http.Client{Timeout: 10 * time.Second},
		log:         log,
		counts:      make(map[string]int),
		batches:     make(map[string][]watcher.Event),
		maxPerMin:   maxPerMin,
		batchWindow: batchWindow,
	}
}

// Send posts a formatted event to Slack, respecting rate limits.
func (s *Sink) Send(ctx context.Context, channel string, event watcher.Event) {
	s.mu.Lock()
	if s.counts[channel] >= s.maxPerMin {
		// Rate limited — buffer for batch.
		s.batches[channel] = append(s.batches[channel], event)
		s.mu.Unlock()
		return
	}
	s.counts[channel]++
	s.mu.Unlock()

	payload := FormatEvent(event)
	s.post(ctx, payload)
}

// FlushBatches sends any accumulated batch summaries.
func (s *Sink) FlushBatches(ctx context.Context) {
	s.mu.Lock()
	batches := s.batches
	s.batches = make(map[string][]watcher.Event)
	s.counts = make(map[string]int) // reset rate limit counters
	s.mu.Unlock()

	for channel, events := range batches {
		if len(events) == 0 {
			continue
		}
		// Group by rig for batch summaries.
		byRig := make(map[string][]watcher.Event)
		for _, e := range events {
			byRig[e.Rig()] = append(byRig[e.Rig()], e)
		}
		for rig, rigEvents := range byRig {
			payload := FormatBatch(rigEvents, rig)
			s.post(ctx, payload)
			s.log.Info("flushed batch", "channel", channel, "rig", rig, "count", len(rigEvents))
		}
	}
}

func (s *Sink) post(ctx context.Context, payload Payload) {
	body, err := json.Marshal(payload)
	if err != nil {
		s.log.Error("marshal payload", "err", err)
		return
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
		if err != nil {
			s.log.Error("create request", "err", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == 429 {
			// Rate limited by Slack — back off.
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error (%d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return // success
		}

		s.log.Warn("unexpected status", "status", resp.StatusCode)
		return
	}

	s.log.Error("webhook failed after retries", "err", lastErr)
}
