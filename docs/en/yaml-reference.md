# Roster YAML Reference

All configuration files declare their type with a `kind:` field.
File naming convention: `{id}.{kind}.yaml`

---

## kind: org

Root container. One per project.

```yaml
# my-company.org.yaml
kind: org
id: my-company
name: My Company
subscribe: [project.requested]
emit: [project.done]
store:
  backend: sqlite        # sqlite | file | memory  (default: file)
  path: .roster/data.db
cron:
  - schedule: "5m"
    event: "heartbeat"
limits:
  max_iterations: 10
  cooldown: "5m"
budget:
  max_per_run: 10.0
  max_daily: 100.0
  max_monthly: 1000.0
```

| Field | Description |
|-------|-------------|
| `subscribe` | Event types this Org listens for |
| `emit` | Event types this Org publishes on completion |
| `store.backend` | State storage backend |
| `store.path` | SQLite file path |
| `cron` | Periodic event emission schedules |
| `limits.max_iterations` | Max times any event type can fire before circuit breaker trips |
| `limits.cooldown` | Duration to suppress after tripping (e.g. `"5m"`) |
| `budget` | Organization-level spending limits (USD) |

> Desks declare membership via the `groups:` field. Groups can nest via `parent:`. The Org does not enumerate its children.

---

## kind: group

Session-sharing scope. Desks declare membership via their `groups` field. Groups can nest via `parent`.

```yaml
# dev-team.group.yaml
kind: group
id: dev-team
name: Dev Team
parent: engineering       # optional: nest inside another group
resources:
  - codebase              # Resources shared by the entire group
```

| Field | Description |
|-------|-------------|
| `parent` | Parent Group ID (for nesting groups) |
| `resources` | Resources shared across the group |

> Groups do not subscribe to or emit events — only desks do. Groups define session-sharing boundaries and resource inheritance.

---

## kind: desk

Execution unit where an Agent sits. Declares group membership via `groups`.

```yaml
# developer.desk.yaml
kind: desk
id: developer
groups:                    # group membership (array — can belong to multiple groups)
  - dev-team
agent: claude-cli         # Agent ID

role: "Senior Go Developer"
goal: "Write Roster code following the single responsibility principle"

skills:
  - go-developer          # Resolved from skills/{name}.yaml
resources:
  - codebase

subscribe: [task.planned]
emit: [code.done]

executor:
  type: sdk               # api | exec | docker | remote | human | sdk
  env:
    ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
    CLAUDE_WORK_DIR: "${MY_PROJECT_DIR}"

session:
  max_entries: 20
  max_knowhow: 10

budget:
  max_per_run: 5.0
  max_daily: 50.0
```

| Field | Description |
|-------|-------------|
| `groups` | Group IDs this desk belongs to (array) |
| `agent` | Agent ID |
| `role` | Agent persona — assembled into the system prompt automatically |
| `goal` | Agent goal — assembled into the system prompt automatically |
| `skills` | Skills to load (included in the prompt) |
| `resources` | Resources the agent can access |
| `subscribe` | Event types to listen for |
| `emit` | Event types to publish (allowlist) |
| `executor` | Execution backend configuration |
| `session.max_entries` | Maximum number of session history entries |
| `session.max_knowhow` | Maximum accumulated knowhow entries (agents write `## Knowhow` sections in output; hub extracts and re-injects them on future runs) |
| `timeout` | Max execution duration (Go duration string, e.g. `"5m"`). Default: `30m`. Human desks have no default timeout |
| `budget.max_per_run` | Max USD per single execution |
| `budget.max_daily` | Max USD per 24h rolling window |
| `budget.max_monthly` | Max USD per 30d rolling window |

### Executor Types

| type | Description | Key env/params |
|------|-------------|----------------|
| `api` | Direct Anthropic / OpenAI / Gemini API call | `sdk`, `model` |
| `exec` | stdin/stdout JSON protocol command | `command` |
| `docker` | Docker container | `image` |
| `remote` | gRPC remote worker | `address` |
| `human` | Handled by a person in the web UI | — |
| `sdk` | Python/Node SDK agent process (gRPC) | — |

### `sdk` executor — Built-in Agents

When using `executor.type: sdk`, the `agent` field references an agent ID registered in the SDK process.

| Agent ID | Description |
|----------|-------------|
| `claude-cli` | Delegates to the `claude` CLI. Assembles `role`/`goal`/`skills` into the system prompt |

`claude-cli` environment variables:

| Variable | Description |
|----------|-------------|
| `CLAUDE_WORK_DIR` | Working directory for claude execution |
| `CLAUDE_MODEL` | Model override |
| `CLAUDE_SYSTEM_PROMPT` | Manual system prompt (ignored if `role`/`goal` are set) |

### Humans as Desks

```yaml
kind: desk
id: approval-gate
groups: [dev-team]
executor:
  type: human
subscribe: [review.done]
emit: [approved, rejected]
```

---

## kind: agent

Logic unit. The unit sold/purchased in the marketplace.

```yaml
# claude-cli.agent.yaml
kind: agent
id: claude-cli
name: Claude CLI
description: "Claude CLI-based agent"
sdk: "pip:roster-sdk"     # pip:{package} | local:{path} | git:{repo}
```

| Field | Description |
|-------|-------------|
| `sdk` | SDK package location |
| `skills` | Default skills for this agent (merged with Desk's skills) |

> **subscribe/emit are declared by the Desk.** The Agent contains pure logic only.

---

## kind: resource

External system connection info. No logic — configuration only.

```yaml
# codebase.resource.yaml
kind: resource
id: codebase
name: Codebase
type: local
config:
  path: ./roster

# figma.resource.yaml
kind: resource
id: figma
type: mcp
mcp: "npx @modelcontextprotocol/server-figma"

# db.resource.yaml
kind: resource
id: db
type: remote
connection: "${DATABASE_URL}"
```

| Field | Description |
|-------|-------------|
| `type` | `local` \| `mcp` \| `remote` \| any string |
| `mcp` | MCP server start command |
| `connection` | DB URL, API endpoint, etc. |
| `config` | Arbitrary key-value passed directly to the agent |
| `watch` | Glob patterns to filter file change events (e.g. `["**/*.go"]`). For `local` resources, the hub watches `config.path` recursively. If omitted, all changes trigger events |

> Resources are configuration only. All interaction logic lives in the Agent.

---

## kind: skill

Prompt package loaded into an agent.

```yaml
# go-developer.skill.yaml
kind: skill
name: go-developer
version: "1.0"
prompt: |
  ## Principles
  - Single responsibility principle
  - No circular dependencies
  - Max 1000 lines per file
```

`.md` files also work — the entire file content is used as the prompt.

**Resolution order:**
1. `skills/{name}.yaml`
2. `{name}.yaml`
3. `knowhow/{name}.yaml`
4. `{name}.md`
5. Remote: `github.com/org/repo/skill-name` or `https://...`

---

## Environment Variable Substitution

Use `${VAR_NAME}` syntax anywhere in YAML values to reference environment variables.

```yaml
env:
  ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
```

---

## Project Structure

```
my-company/
├── my-company.org.yaml
├── dev-team.group.yaml
├── developer.desk.yaml
├── reviewer.desk.yaml
├── human-gate.desk.yaml
├── claude-cli.agent.yaml
├── codebase.resource.yaml
└── skills/
    └── go-developer.skill.yaml
```

All files can live in one folder. The Hub discovers them automatically.

---

## Event Flow

```
Event fires
→ Desks with matching subscribe wake up
→ Desk: Agent executes → signals completion via emit
→ Output goes to session history; data sharing via resources
```
