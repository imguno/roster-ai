# Roster (Go Module)

This directory contains the Roster framework source code — an event-driven AI agent orchestrator written in Go.

```
cmd/roster/        CLI entrypoint
pkg/types/         Domain models (pure types, no external deps)
pkg/sdk/           Port interfaces (Executor, Task)
internal/
  hub/             Central orchestrator — routes events, manages queues
  event/           Event bus, emitters, queue, and routing
  exec/            Executor implementations and desk runner
    runner/        Backends: api, docker, exec, remote
    desk/          Desk execution handler
    sdkproc/       SDK process management (gRPC streaming)
  store/           Persistence: sqlite, file, memory backends
  web/             HTTP API server and React dashboard
  agent/           Skill resolution and knowledge base
  resource/        Resource management
  config/          YAML/JSON project loader
  validate/        Configuration validation
proto/             gRPC protocol definitions (agent, resource, worker)
```

## Build

```bash
go build -o roster ./cmd/roster
```

## Test

```bash
go test ./...
```

## Module

```
github.com/roster-io/roster
```

## CLI

```bash
roster help                  # Show usage
roster init [dir]            # Create organization (--name, --template)
roster hub [--dir .] [--ui :8080] [--store file|sqlite|memory]
                             # Start hub server
roster load <dir> [--hub URL]   # Load org into running hub
roster emit <event> [payload]   # Emit event to hub
roster worker [addr]         # Start remote worker (default :50051)
roster runs [--dir .]        # Show recent runs (--n, --output)
roster logs [--dir .]        # Query logs (--type, --follow, --output)
roster summarize [--dir .]   # Compact session history (--desk, --all)
roster status [--hub URL]    # Show hub status
roster vacuum [--dir .]      # Clean up old data (--keep 7d)
```

Init templates: `product-team`, `content-pipeline`, `code-review`.

---

## Core Concepts

### Organization (Org)

The root container. Exactly one per project. Children (groups, desks) declare membership via their `parent` field.

```yaml
kind: org
id: my-org
name: My Organization
store:
  backend: sqlite       # sqlite | file | memory
  path: .roster/data
resources:
  - shared-db
subscribe:
  - hub.started
emit:
  - hub.started
cron:
  - schedule: "5m"
    event: "heartbeat"
limits:
  max_iterations: 10
  cooldown: "5m"
```

The `limits` field configures the **loop circuit breaker**. When any single event type fires more than `max_iterations` times, the hub suppresses further emissions of that event and logs a `loop.breaker` observation. After `cooldown` elapses, the counter resets and the event type is unblocked. This prevents runaway self-improvement or retry cycles from consuming resources indefinitely.

### Desk

The execution unit — one agent, one job, one set of events.

```yaml
kind: desk
id: code-reviewer
name: Code Reviewer
parent: dev-team          # group membership
agent: reviewer           # local agent ID or remote spec
role: "Senior code reviewer"
goal: "Review PRs for correctness and style"
subscribe:
  - pr.opened
emit:
  - review.done
resources:
  - codebase
skills:
  - code-review-v2
executor:
  type: sdk               # api | exec | docker | remote | human | sdk
session:
  max_entries: 50
```

The `emit` field is an **allowlist**. When set, the hub filters the desk's emitted events — only events listed in `emit` are published. Rejected emissions are logged as `emit.rejected` observations. If all emissions are rejected, the desk falls back to emitting `done`. If `emit` is omitted, the desk can emit any event.

The `timeout` field sets a maximum execution duration for the desk. If the desk exceeds this limit, the hub cancels it and emits a `{deskID}.timed_out` event (plus `{groupID}.{deskID}.timed_out` if the desk belongs to a group). The value is a Go duration string:

```yaml
timeout: "5m"    # cancel after 5 minutes
```

The `agent` field accepts a plain string (local agent ID) or an object for remote agents:

```yaml
agent:
  type: remote
  address: api.vendor.io/agents/ux-v1
  api_key: ${KEY}
```

### Agent

Defines who an agent is — identity and capabilities. All execution concerns live in the Desk.

```yaml
kind: agent
id: reviewer
name: Code Reviewer
sdk: "local:../roster-sdk-python"
skills:
  - code-review-v2
```

### Group

A team container. Desks inside a group share a session scope and can see each other's messages via `GroupHistory`.

```yaml
kind: group
id: dev-team
name: Development Team
parent: my-org
resources:
  - shared-codebase
subscribe:
  - sprint.started
emit:
  - sprint.done
```

### Resource

A connection to an external system. Pure configuration — the agent reads it and handles interaction itself.

```yaml
kind: resource
id: codebase
name: Project Code
type: local
connection: "http://localhost:8080"
mcp: "npx @modelcontextprotocol/server"
watch:
  - file.changed            # events that trigger re-reads
config:
  path: ./src
```

The `watch` field lists event types that indicate the resource has changed. Desks subscribed to those events can react to resource updates.

Resource types: `local`, `mcp`, `remote`, and custom types via `config`.

**Resource inheritance.** A desk inherits resources from its parent group, which inherits from *its* parent, all the way up to the organization. The hub walks the `parent` chain — desk → group → parent group → … → org — and merges all resources into the task. Desk-level resources are resolved first, then each ancestor group's resources, then org-level resources.

**Prompt injection.** The hub automatically appends a `[Resources]` section to the agent's prompt listing each resolved resource with its `description`, `path`, and `connection` fields. Agents see their available resources without any extra configuration. Relative `path` values are resolved to absolute paths using the project directory.

### Skill

Skills teach agents HOW to do things. They are resolved prompts injected into the task.

```yaml
name: code-review-v2
version: "1.0"
prompt: |
  Review the code for correctness, style, and security.
  Flag any OWASP top-10 vulnerabilities.
```

Skill references can be plain names, git paths (`github.com/org/repo/skill-name`), or HTTPS URLs.

### Output

The result of executing a desk. Replaces the old Artifact type — desk outputs are plain text content plus optional metrics. Structured data sharing between desks happens via resources, not output payloads.

```go
type Output struct {
    Content string             `json:"content"`
    Metrics map[string]float64 `json:"metrics,omitempty"`
}
```

---

## Executor Interface

Every execution backend implements the `Executor` interface:

```go
type Executor interface {
    Run(ctx context.Context, task Task) (*types.Output, error)
}
```

The `Task` struct carries everything the executor needs:

| Field | Description |
|---|---|
| `Prompt` | Merged skill prompts + input context, ready to send |
| `Resources` | Available resources with their config |
| `Skills` | Skill name to resolved prompt content |
| `Session` | Persistent conversation history |
| `GroupHistory` | Shared group communication log |
| `Notes` | Key-value note store snapshot |
| `Options` | Executor-specific config (command, image, etc.) |
| `Env` | Environment variables for the subprocess |
| `WorkDir` | Working directory (project root) |

### Executor types

| Type | Description |
|---|---|
| `api` | Call an AI provider directly (Anthropic, OpenAI, Gemini) |
| `exec` | Run a local shell command |
| `docker` | Run inside a Docker container |
| `remote` | Call a remote agent endpoint |
| `human` | Wait for human input via the dashboard |
| `sdk` | Run an SDK agent via gRPC streaming |

---

## Event Bus

The event bus (`internal/event`) routes events between desks, groups, and the hub.

### Publishing events

Use `Bus.Publish` for synchronous delivery or `Bus.PublishAsync` for fire-and-forget:

```go
bus.Publish(ctx, types.Event{Type: "task.created", Source: "api", Payload: data})
bus.PublishAsync(ctx, types.Event{Type: "task.created", Source: "api", Payload: data})
```

### Bus.Emit

`Bus.Emit` is a convenience method for typed result events. It JSON-encodes the payload automatically and publishes asynchronously:

```go
err := bus.Emit(ctx, "my-desk", "analysis.done", map[string]any{
    "score": 0.95,
    "label": "positive",
})
```

Use `Emit` for agent/desk result events that carry structured payloads. Use `Publish`/`PublishAsync` for observation-style events (e.g. `hub.started`).

### DeskEmitter

`DeskEmitter` is a scoped wrapper so desks can emit events without knowing about the bus or their own scope ID:

```go
emitter := event.NewDeskEmitter(bus, "my-desk")
emitter.Emit(ctx, "review.done", result)
```

The hub caches emitters per desk — use `hub.DeskEmitter(deskID)` to get or create one:

```go
em := hub.DeskEmitter("code-reviewer")
em.Emit(ctx, "review.done", reviewPayload)
```

### Event deduplication

Each event has a unique `ID` (set by the hub if empty). The same event ID will not be enqueued twice for the same subscriber, even if multiple routing paths deliver it.

---

## Cron Scheduling

The `Org` type supports a `cron` field for periodic event emission. Each entry fires a timed event on the bus at a fixed interval.

### Configuration

```yaml
kind: org
id: my-org
cron:
  - schedule: "5m"
    event: "heartbeat"
  - schedule: "*/30 * * * *"
    event: "digest.trigger"
    payload: '{"scope": "daily"}'
```

### Schedule formats

| Format | Example | Meaning |
|---|---|---|
| Go duration | `5m`, `1h`, `30s` | Every 5 minutes, 1 hour, 30 seconds |
| Cron expression | `*/30 * * * *` | Every 30 minutes |

Cron schedules start automatically when the hub calls `Start()`. Each entry runs in its own goroutine and stops when the hub context is cancelled.

---

## Dashboard API

The built-in web server exposes a React dashboard and a REST API on port 8080.

| Endpoint | Description |
|---|---|
| `GET /api/organization` | Organization config |
| `GET /api/desks` | All desk configs |
| `GET /api/desks/{id}/profile` | Desk performance metrics (runs, cost, tokens). Supports `?window=1h\|6h\|24h\|7d\|30d` |
| `GET /api/desks/{id}/session` | Desk session history |
| `GET /api/desks/{id}/logs` | Desk execution logs |
| `GET /api/desks/{id}/executor-file` | Executor command file content |
| `GET /api/groups` | All group configs |
| `GET /api/resources` | All resource configs |
| `GET /api/events` | Event log |
| `POST /api/events` | Emit a custom event to the hub |
| `GET /api/stream` | SSE stream for real-time updates |
| `GET /api/queues` | Event queue state |
| `GET /api/runs` | Run history |
| `GET /api/runs/{id}` | Run details |
| `POST /api/runs/{id}/cancel` | Cancel a running execution |
| `GET /api/runs/{id}/events` | Observation events for a run |
| `GET /api/metrics` | Collected metrics. Supports `?desk={id}` filter |
| `POST /api/metrics` | Report metrics for a desk |
| `GET /api/budget` | Budget/usage info |
| `GET /api/warnings` | Configuration warnings |
| `POST /api/human/{id}` | Submit human input for a desk |
| `POST /api/load` | Reload configuration |
| `GET /api/ping` | Health check |
| `GET /api/version` | Server version |
| `GET /health` | Health probe |
| `GET /readiness` | Readiness probe |
| `POST /webhooks/{id}` | Webhook receiver |

### Dashboard features

The React dashboard provides a live graph view of your organization:

- **New Task modal** — click "+ New Task" in the top bar to create a task targeted at the whole org, a specific team, or a single desk. Tasks are emitted as `task.created` or `task.{targetId}` events with source `human`.
- **Human event badges** — events originating from human input (source `human` or `dashboard`) display an "H" badge in the event log.
- **Edge popovers** — click any edge in the graph view to see the events that flowed along that connection, with timestamps and step IDs.
- **Desk retry badges** — desks with retry activity display a visual badge in the graph view.
- **Live polling** — desk popovers auto-refresh session and log data via SSE streaming.

---

## Storage

Configure the storage backend in your organization YAML:

```yaml
store:
  backend: sqlite    # sqlite (default), file, or memory
  path: .roster/data
```

The store provides a unified interface for session history, logs, notes, and metrics — all keyed by scope ID (desk or group).

---

See the [root README](../README.md) for full documentation.
