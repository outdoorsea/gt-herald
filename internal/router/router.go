// Package router maps parsed events to target Slack channels.
package router

import (
	"github.com/outdoorsea/gt-herald/internal/config"
	"github.com/outdoorsea/gt-herald/internal/watcher"
)

// RoutedEvent is an event with its target Slack channel.
type RoutedEvent struct {
	Event   watcher.Event
	Channel string
}

// alertTypes are event types that always go to the alerts channel.
var alertTypes = map[string]bool{
	"crash":           true,
	"session_death":   true,
	"mass_death":      true,
	"escalation_sent": true,
}

// Router maps events to channels using config-driven rules.
type Router struct {
	cfg *config.Config
}

// New creates a router with the given configuration.
func New(cfg *config.Config) *Router {
	return &Router{cfg: cfg}
}

// Route determines the target channel for an event.
// Returns a RoutedEvent and true if the event should be posted,
// or zero value and false if it should be suppressed.
func (r *Router) Route(event watcher.Event) (RoutedEvent, bool) {
	if !r.cfg.ShouldInclude(event.Type) {
		return RoutedEvent{}, false
	}

	channel := r.cfg.ChannelForRig(event.Rig())
	if alertTypes[event.Type] {
		channel = r.cfg.Channels.Alerts
	}

	return RoutedEvent{Event: event, Channel: channel}, true
}
