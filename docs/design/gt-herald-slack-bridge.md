# gt-herald: Gas Town Slack Bridge

**Status**: Spec draft
**Author**: gastown/crew/beercan
**Date**: 2026-03-26

## Overview

gt-herald is a standalone service that bridges Gas Town events to Slack. It
runs as its own process, reads Gas Town's observable outputs (townlog, beads,
git), and posts formatted messages to Slack channels. It has no Go import
dependencies on gastown internals.

**Design principle**: Slack is a mirror, not the source of truth. Beads and
Dolt remain authoritative. Herald reads public artifacts — the same files and
databases any external tool could observe.

## Why standalone?

1. **No coupling** — gastown deploys independently; herald deploys independently
2. **Different failure domain** — Slack outage doesn't affect agent work
3. **Reusable** — any multi-agent system with similar log formats can use it
4. **Simpler auth** — only herald needs Slack tokens, not every agent

## Architecture

```
                    ┌─────────────────────────────┐
                    │        gt-herald             │
                    │                              │
  townlog ──tail──▸ │  Watcher ──▸ Router ──▸ Sink │ ──▸ Slack API
  beads DB ──poll─▸ │                              │
  git log ──poll──▸ │  Config: channel routing,    │
                    │  filters, rate limits         │
                    └─────────────────────────────┘
```

### Components

| Component | Responsibility |
|-----------|---------------|
| **Watcher** | Tail townlog, poll beads DB, watch git log for new events |
| **Router** | Match events to channels using configurable rules |
| **Formatter** | Convert events to Slack Block Kit payloads |
| **Sink** | HTTP client for Slack API with rate limiting and retry |
| **State** | Track cursor positions (last line read, last bead seen) to survive restarts |

## Event Sources

### 1. Townlog (primary)

**File**: `{GT_ROOT}/logs/town.log`
**Format**: `2006-01-02 15:04:05 [type] agent context`
**Method**: `tail -f` equivalent (fsnotify + seek)

Events and their Slack routing:

| Event | Channel | Format |
|-------|---------|--------|
| `spawn` | `#gt-{rig}` | `:hatching_chick: {agent} spawned — {context}` |
| `done` | `#gt-{rig}` | `:white_check_mark: {agent} completed — {context}` |
| `crash` | `#gt-alerts` | `:rotating_light: {agent} crashed — {context}` |
| `session_death` | `#gt-alerts` | `:skull: {agent} session died — {context}` |
| `mass_death` | `#gt-alerts` | `:skull_and_crossbones: Mass death — {context}` |
| `handoff` | `#gt-{rig}` | `:recycle: {agent} cycling` |
| `escalation_sent` | `#gt-alerts` | `:mega: Escalation — {context}` |
| `kill` | (suppressed) | — |
| `nudge` | (suppressed) | — |
| `patrol_*` | (suppressed) | — |

**Suppressed events** are high-frequency, low-signal. Herald filters them by
default but they can be enabled per-channel in config.

### 2. Beads (state changes)

**Source**: Dolt SQL on `{rig}_beads` databases
**Method**: Poll every 30s for recent state transitions
**Query**: `SELECT * FROM issues WHERE updated_at > ? ORDER BY updated_at`

| Transition | Channel | Format |
|------------|---------|--------|
| `open → in_progress` | `#gt-{rig}` | `:construction: {id} claimed by {assignee} — {title}` |
| `* → closed` | `#gt-{rig}` | `:ballot_box_with_check: {id} closed — {title}` |
| `* → blocked` | `#gt-{rig}` | `:no_entry_sign: {id} blocked — {title}` |
| new bead (type=bug) | `#gt-{rig}` | `:bug: {id} filed — {title}` |

### 3. Refinery (merge queue)

**Source**: Dolt SQL on merge_requests table, or `gt feed` output
**Method**: Poll every 60s

| Event | Channel | Format |
|-------|---------|--------|
| MR submitted | `#gt-{rig}` | `:inbox_tray: MR from {agent} — {branch}` |
| MR merged | `#gt-{rig}` | `:merged: {branch} merged to main` |
| MR rejected | `#gt-{rig}` | `:x: MR rejected — {reason}` |

### 4. Faultline (errors)

Faultline already has its own Slack webhook support (`FAULTLINE_SLACK_WEBHOOK`).
Herald should NOT duplicate this. Instead, faultline posts directly to
`#gt-alerts` via its built-in notifier. Herald acknowledges faultline's
channel ownership and avoids posting duplicate error events.

## Configuration

Config file: `{GT_ROOT}/herald/config.yaml` (or `~/.config/gt-herald/config.yaml`)

```yaml
# Slack connection
slack:
  # Bot token (xoxb-...) — needs chat:write scope
  token_env: GT_HERALD_SLACK_TOKEN
  # Fallback: incoming webhook URL (limited — can only post to one channel)
  # webhook_url_env: GT_HERALD_SLACK_WEBHOOK

# Gas Town connection
gastown:
  root: /Users/jeremy/Documents/gt
  dolt_port: 3307

# Channel routing
channels:
  alerts: "#gt-alerts"
  default: "#gt-ops"
  rigs:
    gastown: "#gt-gastown"
    vitalitek: "#gt-vitalitek"
    faultline: "#gt-faultline"
    # Rigs without explicit mapping use {default}

# Event filters — which events to post
filters:
  townlog:
    include: [spawn, done, crash, session_death, mass_death, handoff, escalation_sent]
    # exclude: [nudge, patrol_started, patrol_complete, polecat_checked]
  beads:
    include: [opened, closed, blocked, claimed]
  refinery:
    include: [submitted, merged, rejected]

# Rate limiting
rate_limit:
  # Max messages per channel per minute
  per_channel: 10
  # Burst buffer (allow short spikes)
  burst: 5
  # When rate-limited, batch events into a single summary message
  batch_window: 30s

# Digest mode — instead of real-time, post periodic summaries
# digest:
#   enabled: false
#   interval: 15m
#   channel: "#gt-digest"

# State persistence (cursor positions survive restarts)
state_file: /Users/jeremy/Documents/gt/herald/state.json
```

## Slack Message Design

### Agent Lifecycle (townlog)

```
:hatching_chick: *gastown/polecats/Toast* spawned
> Working on `gt-h8x` — fix submodule init after monorepo migration
> _12:34 PM_
```

### Crash Alert

```
:rotating_light: *vitalitek/polecats/onyx* crashed
> Session died unexpectedly after 45m
> Last hook: `vi-rpa`
> _12:34 PM_
```

### Bead Closed

```
:ballot_box_with_check: `gt-6g2` closed
> gt sling fails after monorepo migration: stale submodule init persists
> Closed by gastown/crew/beercan
> _12:34 PM_
```

### MR Merged

```
:merged: Merged to main — `polecat/Toast/gt-h8x`
> fix: guard submodule init for monorepo migration (gt-6g2)
> +27 -9 across 2 files
> _12:34 PM_
```

### Rate-Limited Batch

When events exceed the rate limit, batch into a summary:

```
:fast_forward: *5 events in the last 30s* (gastown)
> :hatching_chick: 2 polecats spawned (quartz, jasper)
> :white_check_mark: 1 polecat done (onyx → vi-rpa)
> :ballot_box_with_check: 2 beads closed (vi-3sm, vi-0w4)
```

## Thread Strategy

- **Convoys** get a thread: first message is convoy creation, updates append
- **Beads** get a thread if they generate 3+ events (opened → claimed → closed)
- **Crashes** are top-level (never threaded — must be visible)
- **Routine lifecycle** (spawn/done) are top-level but can be batched

Herald tracks `{bead_id → slack_ts}` and `{convoy_id → slack_ts}` mappings
in its state file to reply in threads.

## Implementation Plan

### Phase 1: Townlog tail + Slack webhook (MVP)

Minimal viable bridge. Single binary, watches townlog, posts to one Slack
channel via incoming webhook.

```
gt-herald watch --config herald/config.yaml
```

**Scope**: townlog events only, webhook (not bot token), no threading, no
beads polling.

**Deliverable**: crashes and completions appear in `#gt-alerts` within seconds.

### Phase 2: Multi-channel routing + beads polling

Switch from webhook to Bot API token. Route events to per-rig channels.
Add beads state polling for bead lifecycle events.

**Deliverable**: each rig gets its own channel with relevant activity.

### Phase 3: Refinery + threading + rate limiting

Add MR lifecycle events. Thread convoy updates. Implement rate limiting
with batch summaries.

**Deliverable**: full operational visibility in Slack.

### Phase 4: Slash commands (inbound)

Add a Slack Bolt handler for inbound commands:

```
/gt status gastown        → rig health summary
/gt beads ready           → available work
/gt sling gt-h8x gastown  → dispatch work
/gt estop                 → emergency stop
```

**Deliverable**: operators can manage Gas Town from Slack.

## Deployment

Herald runs as a long-lived process alongside the Gas Town daemon. It could
be managed by the daemon as a patrol (like dolt_server) or run independently
via systemd/launchd.

**Option A**: Daemon-managed (add to daemon.json patrols)
- Pro: lifecycle managed with everything else
- Con: couples herald to daemon restarts

**Option B**: Independent process (launchd plist or tmux session)
- Pro: fully decoupled
- Con: separate monitoring

Recommend **Option B** for the "not directly integrated" requirement. A
simple launchd plist or a persistent tmux session (`gt-herald`) works.

## Repository Structure

```
gt-herald/
├── cmd/
│   └── gt-herald/
│       └── main.go
├── internal/
│   ├── config/       # YAML config loading
│   ├── watcher/
│   │   ├── townlog.go   # Tail townlog file
│   │   ├── beads.go     # Poll beads DB via SQL
│   │   └── refinery.go  # Poll merge queue
│   ├── router/       # Event → channel routing
│   ├── formatter/    # Event → Slack Block Kit
│   ├── slack/        # Slack API client (webhook + Bot API)
│   └── state/        # Cursor persistence
├── config.example.yaml
├── go.mod
└── README.md
```

**go.mod**: `module github.com/outdoorsea/gt-herald` — its own module, no
gastown dependency. Uses `github.com/go-sql-driver/mysql` for Dolt queries
and `github.com/fsnotify/fsnotify` for file watching.

## Non-Goals

- **Not a chatbot**: Herald doesn't have conversations. It's a one-way bridge
  (plus slash commands in Phase 4).
- **Not a replacement for gt mail**: Agent-to-agent communication stays in
  Dolt. Herald is for human visibility.
- **Not a dashboard**: For rich UI, use the web feed (`gt feed`). Herald is
  for passive monitoring in a channel you're already watching.
- **No agent identity in Slack**: Agents don't get Slack accounts. Herald
  posts on their behalf with their name in the message.

## Security

- Slack bot token stored in env var, never in config files
- Herald has **read-only** access to Gas Town (tails logs, reads Dolt)
- Slash commands (Phase 4) authenticate via Slack signing secret and map
  to the overseer's permissions — no privilege escalation
- Herald never writes to beads, townlog, or git

## Open Questions

1. **Channel per rig vs. shared channel?** For a small number of rigs (10),
   per-rig channels work. At 50+ rigs, a shared channel with rig tags and
   filtering may be better.

2. **Digest vs. real-time?** Some teams prefer a periodic summary (every 15m)
   over a stream of events. Config supports both; which should be default?

3. **Slack Connect for external contributors?** If community PRs trigger
   polecat work, should fix-merge completions post to a public channel?
