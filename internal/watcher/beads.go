package watcher

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// BeadsWatcher polls Dolt databases for bead state changes.
type BeadsWatcher struct {
	dsn      string // e.g., "root@tcp(127.0.0.1:3307)/"
	rigs     []string
	interval time.Duration
	log      *slog.Logger
	events   chan Event
	// lastSeen tracks the most recent updated_at per rig to avoid re-emitting.
	lastSeen map[string]time.Time
}

// NewBeadsWatcher creates a watcher that polls bead state changes from Dolt.
func NewBeadsWatcher(host string, port int, rigs []string, interval time.Duration, log *slog.Logger) *BeadsWatcher {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/", host, port)
	return &BeadsWatcher{
		dsn:      dsn,
		rigs:     rigs,
		interval: interval,
		log:      log,
		events:   make(chan Event, 100),
		lastSeen: make(map[string]time.Time),
	}
}

// Events returns the channel of bead state change events.
func (w *BeadsWatcher) Events() <-chan Event {
	return w.events
}

// SetLastSeen restores cursor positions from saved state.
func (w *BeadsWatcher) SetLastSeen(cursors map[string]time.Time) {
	for k, v := range cursors {
		w.lastSeen[k] = v
	}
}

// GetLastSeen returns current cursor positions for state persistence.
func (w *BeadsWatcher) GetLastSeen() map[string]time.Time {
	result := make(map[string]time.Time, len(w.lastSeen))
	for k, v := range w.lastSeen {
		result[k] = v
	}
	return result
}

// Start begins polling. Blocks until ctx is cancelled.
func (w *BeadsWatcher) Start(ctx context.Context) error {
	db, err := sql.Open("mysql", w.dsn)
	if err != nil {
		return fmt.Errorf("connecting to Dolt: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Initial poll.
	w.pollAll(ctx, db)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.events)
			return nil
		case <-ticker.C:
			w.pollAll(ctx, db)
		}
	}
}

func (w *BeadsWatcher) pollAll(ctx context.Context, db *sql.DB) {
	for _, rig := range w.rigs {
		if ctx.Err() != nil {
			return
		}
		w.pollRig(ctx, db, rig)
	}
}

func (w *BeadsWatcher) pollRig(ctx context.Context, db *sql.DB, rig string) {
	dbName := rig + "_beads"
	since := w.lastSeen[rig]
	if since.IsZero() {
		// First poll: only look at last 5 minutes to avoid flooding.
		since = time.Now().Add(-5 * time.Minute)
	}

	query := fmt.Sprintf(
		"SELECT id, title, status, assignee, updated_at FROM `%s`.issues WHERE updated_at > ? ORDER BY updated_at ASC LIMIT 50",
		dbName,
	)

	rows, err := db.QueryContext(ctx, query, since.Format("2006-01-02 15:04:05"))
	if err != nil {
		// Database might not exist or be temporarily unavailable.
		w.log.Debug("beads poll failed", "rig", rig, "err", err)
		return
	}
	defer rows.Close()

	var maxUpdated time.Time
	for rows.Next() {
		var id, title, status, assignee string
		var updatedAt time.Time
		if err := rows.Scan(&id, &title, &status, &assignee, &updatedAt); err != nil {
			w.log.Warn("scan row", "rig", rig, "err", err)
			continue
		}

		eventType := beadEventType(status)
		if eventType == "" {
			continue
		}

		agent := assignee
		if agent == "" {
			agent = rig
		}

		w.events <- Event{
			Timestamp: updatedAt,
			Type:      eventType,
			Agent:     agent,
			Context:   fmt.Sprintf("`%s` %s", id, truncate(title, 100)),
		}

		if updatedAt.After(maxUpdated) {
			maxUpdated = updatedAt
		}
	}

	if !maxUpdated.IsZero() {
		w.lastSeen[rig] = maxUpdated
	}
}

// beadEventType maps bead status to a herald event type for channel routing.
func beadEventType(status string) string {
	switch status {
	case "closed":
		return "bead_closed"
	case "in_progress":
		return "bead_claimed"
	case "blocked":
		return "bead_blocked"
	case "open":
		return "bead_opened"
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
