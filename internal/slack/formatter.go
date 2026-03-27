// Package slack provides Slack webhook and Block Kit formatting for gt-herald.
package slack

import (
	"fmt"
	"time"

	"github.com/outdoorsea/gt-herald/internal/watcher"
)

// Payload is a Slack Block Kit message.
type Payload struct {
	Blocks []Block `json:"blocks"`
}

// Block is a single Slack Block Kit block.
type Block struct {
	Type     string  `json:"type"`
	Text     *Text   `json:"text,omitempty"`
	Elements []Block `json:"elements,omitempty"`
}

// Text is a Slack text object.
type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// FormatEvent converts a watcher event into a Slack Block Kit payload.
func FormatEvent(event watcher.Event) Payload {
	emoji := emojiFor(event.Type)
	header := fmt.Sprintf("%s *%s* %s", emoji, event.Agent, event.Type)

	blocks := []Block{
		{Type: "section", Text: &Text{Type: "mrkdwn", Text: header}},
	}

	if event.Context != "" {
		blocks = append(blocks, Block{
			Type: "context",
			Elements: []Block{
				{Type: "mrkdwn", Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("> %s", event.Context)}},
			},
		})
	}

	blocks = append(blocks, Block{
		Type: "context",
		Elements: []Block{
			{Type: "mrkdwn", Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("_%s_", event.Timestamp.Format(time.Kitchen))}},
		},
	})

	return Payload{Blocks: blocks}
}

// FormatBatch creates a summary message for batched events.
func FormatBatch(events []watcher.Event, rig string) Payload {
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.Type]++
	}

	header := fmt.Sprintf(":fast_forward: *%d events in the last batch* (%s)", len(events), rig)
	lines := ""
	for typ, count := range counts {
		lines += fmt.Sprintf("\n> %s %d× %s", emojiFor(typ), count, typ)
	}

	return Payload{
		Blocks: []Block{
			{Type: "section", Text: &Text{Type: "mrkdwn", Text: header + lines}},
		},
	}
}

func emojiFor(eventType string) string {
	switch eventType {
	case "spawn":
		return ":hatching_chick:"
	case "done":
		return ":white_check_mark:"
	case "crash":
		return ":rotating_light:"
	case "session_death":
		return ":skull:"
	case "mass_death":
		return ":skull_and_crossbones:"
	case "handoff":
		return ":recycle:"
	case "escalation_sent":
		return ":mega:"
	case "kill":
		return ":knife:"
	case "wake":
		return ":sunny:"
	case "bead_closed":
		return ":ballot_box_with_check:"
	case "bead_claimed":
		return ":construction:"
	case "bead_blocked":
		return ":no_entry_sign:"
	case "bead_opened":
		return ":bug:"
	default:
		return ":information_source:"
	}
}
