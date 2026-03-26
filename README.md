# gt-herald

Gas Town Slack bridge — broadcasts agent lifecycle events to Slack channels.

gt-herald is a standalone service that watches Gas Town's observable outputs
(townlog, beads, git) and posts formatted messages to Slack. It has no Go
import dependencies on gastown internals.

## Status

Under development. See [design spec](docs/design/gt-herald-slack-bridge.md) for the full plan.

## Quick Start

```bash
# Configure
export GT_HERALD_SLACK_WEBHOOK="https://hooks.slack.com/services/..."
cp config.example.yaml herald.yaml
# Edit herald.yaml with your GT_ROOT and channel settings

# Run
go build -o gt-herald ./cmd/gt-herald
./gt-herald watch --config herald.yaml
```

## License

MIT
