// Package watcher provides file-watching event sources for gt-herald.
package watcher

import "time"

// Event represents a parsed townlog event.
type Event struct {
	Timestamp time.Time
	Type      string // spawn, done, crash, session_death, mass_death, handoff, etc.
	Agent     string // e.g., "gastown/crew/beercan", "vitalitek/polecats/onyx"
	Context   string // Additional context (issue ID, error message, etc.)
}

// Rig extracts the rig name from the agent string.
// "gastown/crew/beercan" → "gastown"
// "vitalitek/polecats/onyx" → "vitalitek"
// "mayor" → "mayor"
func (e Event) Rig() string {
	for i, c := range e.Agent {
		if c == '/' {
			return e.Agent[:i]
		}
	}
	return e.Agent
}
