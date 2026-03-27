package watcher

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TownlogWatcher tails a Gas Town townlog file and emits parsed events.
type TownlogWatcher struct {
	path   string
	offset int64
	log    *slog.Logger
	events chan Event
}

// NewTownlogWatcher creates a watcher for the given townlog file path.
func NewTownlogWatcher(path string, offset int64, log *slog.Logger) *TownlogWatcher {
	return &TownlogWatcher{
		path:   path,
		offset: offset,
		log:    log,
		events: make(chan Event, 100),
	}
}

// Events returns the channel of parsed events.
func (w *TownlogWatcher) Events() <-chan Event {
	return w.events
}

// Offset returns the current byte offset (for state persistence).
func (w *TownlogWatcher) Offset() int64 {
	return w.offset
}

// Start begins watching the townlog file. Blocks until ctx is cancelled.
func (w *TownlogWatcher) Start(ctx context.Context) error {
	// Read any existing content from offset first.
	if err := w.readNewLines(); err != nil {
		w.log.Warn("initial read failed", "err", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(w.path); err != nil {
		// File might not exist yet — watch the directory instead.
		dir := w.path[:strings.LastIndex(w.path, "/")]
		if dirErr := watcher.Add(dir); dirErr != nil {
			return fmt.Errorf("watching %s: %w", w.path, err)
		}
		w.log.Info("watching directory for file creation", "dir", dir)
	}

	// Poll interval as fallback for missed fsnotify events.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.events)
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if err := w.readNewLines(); err != nil {
					w.log.Warn("read failed", "err", err)
				}
				// If file was recreated, re-add the watch.
				if event.Op&fsnotify.Create != 0 && event.Name == w.path {
					_ = watcher.Add(w.path)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.log.Warn("fsnotify error", "err", err)

		case <-ticker.C:
			if err := w.readNewLines(); err != nil {
				w.log.Warn("poll read failed", "err", err)
			}
		}
	}
}

func (w *TownlogWatcher) readNewLines() error {
	f, err := os.Open(w.path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Detect truncation (log rotation).
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < w.offset {
		w.log.Info("file truncated, resetting offset", "old", w.offset, "new_size", info.Size())
		w.offset = 0
	}

	if info.Size() == w.offset {
		return nil // No new data.
	}

	if _, err := f.Seek(w.offset, io.SeekStart); err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if event, ok := ParseLogLine(line); ok {
			w.events <- event
		}
	}

	// bufio.Scanner reads ahead, so use file size since we scanned to EOF.
	w.offset = info.Size()

	return scanner.Err()
}

// ParseLogLine parses a single townlog line.
// Format: "2006-01-02 15:04:05 [type] agent context"
func ParseLogLine(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if len(line) < 22 { // minimum: "2006-01-02 15:04:05 [x]"
		return Event{}, false
	}

	// Parse timestamp (first 19 chars).
	ts, err := time.Parse("2006-01-02 15:04:05", line[:19])
	if err != nil {
		return Event{}, false
	}

	rest := line[20:] // skip space after timestamp

	// Parse [type].
	if rest[0] != '[' {
		return Event{}, false
	}
	closeBracket := strings.IndexByte(rest, ']')
	if closeBracket < 0 {
		return Event{}, false
	}
	eventType := rest[1:closeBracket]

	rest = rest[closeBracket+1:]
	rest = strings.TrimLeft(rest, " ")

	// Split agent and context.
	agent := rest
	context := ""
	if idx := strings.IndexByte(rest, ' '); idx >= 0 {
		agent = rest[:idx]
		context = strings.TrimSpace(rest[idx+1:])
	}

	return Event{
		Timestamp: ts,
		Type:      eventType,
		Agent:     agent,
		Context:   context,
	}, true
}
