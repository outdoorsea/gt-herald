package watcher

import (
	"testing"
	"time"
)

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		line    string
		want    Event
		wantOK  bool
	}{
		{
			line: "2026-03-26 12:34:56 [spawn] gastown/polecats/Toast gt-h8x",
			want: Event{
				Timestamp: time.Date(2026, 3, 26, 12, 34, 56, 0, time.UTC),
				Type:      "spawn",
				Agent:     "gastown/polecats/Toast",
				Context:   "gt-h8x",
			},
			wantOK: true,
		},
		{
			line: "2026-03-26 14:00:00 [crash] vitalitek/polecats/onyx Session died unexpectedly after 45m",
			want: Event{
				Timestamp: time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC),
				Type:      "crash",
				Agent:     "vitalitek/polecats/onyx",
				Context:   "Session died unexpectedly after 45m",
			},
			wantOK: true,
		},
		{
			line: "2026-03-26 14:00:00 [done] gastown/crew/beercan",
			want: Event{
				Timestamp: time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC),
				Type:      "done",
				Agent:     "gastown/crew/beercan",
				Context:   "",
			},
			wantOK: true,
		},
		{
			line:   "",
			wantOK: false,
		},
		{
			line:   "not a log line",
			wantOK: false,
		},
		{
			line:   "2026-03-26 14:00:00 no brackets",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got, ok := ParseLogLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ParseLogLine(%q): ok=%v, want %v", tt.line, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %q, want %q", got.Type, tt.want.Type)
			}
			if got.Agent != tt.want.Agent {
				t.Errorf("Agent = %q, want %q", got.Agent, tt.want.Agent)
			}
			if got.Context != tt.want.Context {
				t.Errorf("Context = %q, want %q", got.Context, tt.want.Context)
			}
			if !got.Timestamp.Equal(tt.want.Timestamp) {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
			}
		})
	}
}

func TestEvent_Rig(t *testing.T) {
	tests := []struct {
		agent string
		want  string
	}{
		{"gastown/crew/beercan", "gastown"},
		{"vitalitek/polecats/onyx", "vitalitek"},
		{"mayor", "mayor"},
		{"faultline/witness", "faultline"},
	}
	for _, tt := range tests {
		e := Event{Agent: tt.agent}
		if got := e.Rig(); got != tt.want {
			t.Errorf("Event{Agent: %q}.Rig() = %q, want %q", tt.agent, got, tt.want)
		}
	}
}
