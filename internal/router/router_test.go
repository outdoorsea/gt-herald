package router

import (
	"testing"

	"github.com/outdoorsea/gt-herald/internal/config"
	"github.com/outdoorsea/gt-herald/internal/watcher"
)

func testConfig() *config.Config {
	return &config.Config{
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
	}
}

func TestRoute_CrashToAlerts(t *testing.T) {
	r := New(testConfig())
	event := watcher.Event{Type: "crash", Agent: "gastown/polecats/Toast"}
	routed, ok := r.Route(event)
	if !ok {
		t.Fatal("crash should not be suppressed")
	}
	if routed.Channel != "#gt-alerts" {
		t.Errorf("crash should route to alerts, got %q", routed.Channel)
	}
}

func TestRoute_SpawnToRigChannel(t *testing.T) {
	r := New(testConfig())
	event := watcher.Event{Type: "spawn", Agent: "gastown/polecats/Toast"}
	routed, ok := r.Route(event)
	if !ok {
		t.Fatal("spawn should not be suppressed")
	}
	if routed.Channel != "#gt-gastown" {
		t.Errorf("gastown spawn should route to #gt-gastown, got %q", routed.Channel)
	}
}

func TestRoute_UnknownRigToDefault(t *testing.T) {
	r := New(testConfig())
	event := watcher.Event{Type: "done", Agent: "faultline/polecats/alpha"}
	routed, ok := r.Route(event)
	if !ok {
		t.Fatal("done should not be suppressed")
	}
	if routed.Channel != "#gt-ops" {
		t.Errorf("unknown rig should route to default, got %q", routed.Channel)
	}
}

func TestRoute_SuppressedEventType(t *testing.T) {
	r := New(testConfig())
	event := watcher.Event{Type: "nudge", Agent: "gastown/crew/beercan"}
	_, ok := r.Route(event)
	if ok {
		t.Error("nudge should be suppressed (not in include list)")
	}
}

func TestRoute_SessionDeathToAlerts(t *testing.T) {
	r := New(testConfig())
	event := watcher.Event{Type: "session_death", Agent: "vitalitek/polecats/onyx"}
	routed, ok := r.Route(event)
	if !ok {
		t.Fatal("session_death should not be suppressed")
	}
	if routed.Channel != "#gt-alerts" {
		t.Errorf("session_death should route to alerts, got %q", routed.Channel)
	}
}
