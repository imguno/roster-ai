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
```

| Field | Description |
|-------|-------------|
| `subscribe` | Event types this Org listens for |
| `emit` | Event types this Org publishes on completion |
| `store.backend` | State storage backend |
| `store.path` | SQLite file path |

> Groups and Desks declare membership via the `parent:` field. The Org does not enumerate its children.

---

## kind: group

Team container. Declares membership via `parent`.

```yaml
# dev-team.group.yaml
kind: group
id: dev-team
name: Dev Team
parent: my-company        # Org ID or parent Group ID
subscribe: [task.created]
emit: [task.done]
resources:
  - codebase              # Resources shared by the entire group
```

| Field | Description |
|-------|-------------|
| `parent` | Parent Org or Group ID |
| `subscribe` | Event types this Group listens for |
| `emit` | Completion events for this Group (done when any member emits one) |
| `resources` | Resources shared across the group |

> Completion decisions are made by the Agent by reading Notes — no dispatch or lead orchestration.

---

## kind: desk

Execution unit where an Agent sits. Declares membership via `parent`.

```yaml
# developer.desk.yaml
kind: desk
id: developer
parent: dev-team          # Group ID or Org ID
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
```

| Field | Description |
|-------|-------------|
| `parent` | Parent Group or Org ID |
| `agent` | Agent ID |
| `role` | Agent persona — assembled into the system prompt automatically |
| `goal` | Agent goal — assembled into the system prompt automatically |
| `skills` | Skills to load (included in the prompt) |
| `resources` | Resources the agent can access |
| `subscribe` | Event types to listen for |
| `emit` | Event types to publish |
| `executor` | Execution backend configuration |
| `session.max_entries` | Maximum number of session history entries |

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
parent: dev-team
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
→ Desks/Groups with matching subscribe wake up
→ Desk: Agent executes → signals completion via emit
→ Group: Propagates to internal bus → done when any member emits the Group's emit event
```

Group completion decisions are made by the Agent reading Notes directly.
