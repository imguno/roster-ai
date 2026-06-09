# Roster YAML Schema Reference

Every Roster config file declares a `kind` field. The hub discovers and loads files from any directory structure based on this field alone.

## Valid Kinds

| Kind | Description |
|---|---|
| `organization` | Top-level system: groups, routing, store, defaults |
| `group` | Team of desks with shared event stream |
| `desk` | Independent actor: receives events, executes work, emits events |
| `agent` | Identity: role description, skills, knowhow |
| `resource` | Connection to an external system (GitHub, Slack, custom) |
| `policy` | Operational rules: retry, timeout, budget, escalation |
| `skill` | Prompt fragment (YAML with `name`, `version`, `prompt`) |

## File Naming and ID Derivation

IDs are derived automatically from file names when `id:` is omitted (the common case):

| File path | Derived ID | Rule |
|---|---|---|
| `desks/reviewer.yaml` | `reviewer` | Stem of file name |
| `desks/reviewer/desk.yaml` | `reviewer` | When stem equals the kind, use parent directory name |
| `groups/dev-team.yaml` | `dev-team` | Same stem rule |
| `agents/senior-dev/agent.yaml` | `senior-dev` | Same parent-dir rule |

The rule: strip the extension to get the stem. If the stem equals the kind name (e.g. a file named `desk.yaml` for kind `desk`), use the parent directory name instead.

## Implicit Agent Binding

When a desk's name matches an agent's ID, the agent is auto-bound. No explicit `agent:` field needed.

Example: a desk with ID `reviewer` and an agent with ID `reviewer` are automatically linked. The desk's `agent` field is only needed when the names differ or when defining the agent inline.

## Defaults Inheritance

Desk configuration inherits defaults in this order (later overrides earlier):

```
Organization defaults  ->  Group defaults  ->  Desk-level config
```

Both `organization.defaults` and `group.defaults` use the same `DeskDefaults` structure:

| Field | YAML | Type | Description |
|---|---|---|---|
| Executor | `executor` | ExecutorConfig | Default executor for all desks |
| Policy | `policy` | string | Default policy reference |
| Tags | `tags` | string[] | Default tags applied to desks |

---

## Organization

One per project. Defines groups, routing, storage, and defaults.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Kind | `kind` | string | yes | `"organization"` |
| ID | `id` | string | | Auto-derived from file name |
| Name | `name` | string | | Human-readable name |
| Description | `description` | string | | |
| Groups | `groups` | string[] | | Group IDs belonging to this org |
| Resources | `resources` | string[] | | Org-level resource IDs |
| Routing | `routing` | RoutingRule[] | | Event routing table |
| Store | `store` | StoreConfig | | Storage backend config |
| Defaults | `defaults` | DeskDefaults | | Fallback values for all desks |

### RoutingRule

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| On | `on` | string | yes | Event type to match |
| To | `to` | string | yes | Target group or desk ID |
| When | `when` | string | | Conditional expression for routing |

### StoreConfig

| Field | YAML | Type | Description |
|---|---|---|---|
| Backend | `backend` | string | `"file"` (default), `"sqlite"`, `"memory"` |
| Path | `path` | string | Data dir or db path. Default: `.roster/data` |

### Example

```yaml
kind: organization
name: engineering
description: Engineering organization with three teams

groups:
  - strategy-team
  - dev-team
  - ops-team

resources:
  - github-app
  - slack-org

defaults:
  executor:
    type: exec
    params:
      command: scripts/claude-code.sh
    env:
      CLAUDE_MODEL: claude-sonnet-4-6
  policy: standard
  tags: [team-member]

store:
  backend: sqlite
  path: .roster/data/roster.db

routing:
  - on: task.created
    to: strategy-team
  - on: plan.ready
    to: dev-team
  - on: code.ready
    to: ops-team
    when: "event.labels contains 'approved'"
```

---

## Group

A team of desks with a shared event stream. Every desk sees what the others produce.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Kind | `kind` | string | yes | `"group"` |
| ID | `id` | string | | Auto-derived from file name |
| Name | `name` | string | | Human-readable name |
| Description | `description` | string | | |
| Lead | `lead` | GroupLead | | Coordinator desk config |
| Desks | `desks` | string[] | | Member desk IDs |
| Groups | `groups` | string[] | | Nested sub-group IDs |
| Resources | `resources` | string[] | | Group-level resource IDs |
| Subscribe | `subscribe` | string[] | | Event types this group listens to |
| Emit | `emit` | string[] | | Event types this group produces |
| Cron | `cron` | string | | Cron schedule (e.g. `"0 */3 * * *"`) |
| Policy | `policy` | string | | Policy reference |
| Dispatch | `dispatch` | string | | `"sequential"` (default), `"parallel"`, `"conversation"` |
| Triggers | `triggers` | TriggerConfig[] | | Automated event sources |
| Defaults | `defaults` | DeskDefaults | | Override org defaults for this group's desks |

### GroupLead

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Desk | `desk` | string | yes | Lead desk ID |
| Position | `position` | string | | `"both"` (default), `"first"`, `"last"` |

**Lead positions:**

| Position | Pattern |
|---|---|
| `both` (default) | lead plans -> members work -> lead synthesizes |
| `first` | lead decomposes -> members execute |
| `last` | members work -> lead synthesizes |
| _(omitted)_ | No lead; members work directly |

**Dispatch modes:**

| Mode | Description |
|---|---|
| `sequential` | Members run one at a time, each seeing prior outputs (default) |
| `parallel` | All members run concurrently with the same input, results merged |
| `conversation` | Two rounds of sequential execution; members respond to each other |

### Example

```yaml
kind: group
name: dev-team
description: Development team with lead, implementers, and reviewer

lead:
  desk: architect
  position: both

desks:
  - implementer-a
  - implementer-b
  - reviewer

groups:
  - testing-subteam

resources:
  - codebase

subscribe:
  - plan.ready
emit:
  - code.ready

cron: "0 9 * * 1"

policy: team-policy
dispatch: sequential

triggers:
  - type: poll
    url: https://ci.example.com/status
    interval: 5m
    event: ci.updated

defaults:
  executor:
    type: exec
    params:
      command: scripts/claude-code.sh
  policy: careful
  tags: [dev]
```

---

## Desk

An independent actor. Receives events, does one job, emits events.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Kind | `kind` | string | yes | `"desk"` |
| ID | `id` | string | | Auto-derived from file name |
| Name | `name` | string | | Human-readable name |
| Description | `description` | string | | |
| Executor | `executor` | ExecutorConfig | yes | How this desk runs |
| Concurrency | `concurrency` | ConcurrencyConfig | | Parallel execution control |
| Subscribe | `subscribe` | string[] | | Event types this desk listens to |
| Emit | `emit` | string[] | | Event types this desk produces |
| Cron | `cron` | string | | Cron schedule (e.g. `"*/30 * * * *"`) |
| Resources | `resources` | string[] | | Private resource IDs bound to this desk |
| Tags | `tags` | string[] | | Role-based permission tags (e.g. `["backend", "senior"]`) |
| Policy | `policy` | string | | Policy reference |
| Triggers | `triggers` | TriggerConfig[] | | Automated event sources |
| Session | `session` | SessionConfig | | Session history behavior |

Note: The `agent` field is not a YAML field (yaml tag is `"-"`). Agent binding is done by name matching or inline definition (see Implicit Agent Binding above).

### ExecutorConfig

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Type | `type` | string | yes | `"api"`, `"exec"`, `"docker"`, `"remote"`, `"human"` |
| SDK | `sdk` | string | | AI SDK when type=api: `"anthropic"`, `"openai"`, `"gemini"` |
| Address | `address` | string | | Endpoint for remote executor |
| Params | `params` | map[string]string | | Executor-specific parameters |
| Env | `env` | map[string]string | | Environment variables |

**Executor types:**

| Type | Description | Key params/fields |
|---|---|---|
| `api` | Built-in AI SDK | `sdk`, plus model/settings in `params` |
| `exec` | Arbitrary command via stdin/stdout | `command` in `params`, `env` |
| `docker` | Docker container | `image` in `params` |
| `remote` | Remote worker via gRPC | `address` |
| `human` | Human participant via web UI | |

### ConcurrencyConfig

| Field | YAML | Type | Description |
|---|---|---|---|
| Mode | `mode` | string | `"queue"` (default), `"spawn"`, `"reject"` |
| Max | `max` | int | Max parallel workers (for `spawn` mode) |

**Concurrency modes:**

| Mode | Description |
|---|---|
| `queue` | Queue requests (default) |
| `spawn` | Spawn parallel workers up to `max` |
| `reject` | Reject when busy |

### TriggerConfig

| Field | YAML | Type | Description |
|---|---|---|---|
| Type | `type` | string | `"exec"`, `"poll"` |
| Command | `command` | string | Command to run (exec type). Fires event on exit code 0 |
| URL | `url` | string | URL to GET (poll type). Fires event on status 200 |
| Interval | `interval` | string | Time between checks (default: `"30s"`) |
| Event | `event` | string | Event type to emit when triggered |

### SessionConfig

| Field | YAML | Type | Description |
|---|---|---|---|
| MaxEntries | `max_entries` | *int | Max session entries loaded as context. Default: 40. Set to 0 to disable |

### Example

```yaml
kind: desk
name: reviewer
description: Reviews code changes for quality and security

executor:
  type: exec
  params:
    command: scripts/claude-code.sh
  env:
    CLAUDE_MODEL: claude-sonnet-4-6

concurrency:
  mode: spawn
  max: 3

subscribe:
  - code.submitted
emit:
  - review.done

cron: "0 9 * * *"

resources:
  - codebase
  - github-app

tags:
  - backend
  - senior

policy: careful

triggers:
  - type: exec
    command: scripts/check-new-prs.sh
    interval: 2m
    event: pr.needs_review
  - type: poll
    url: https://ci.example.com/pending-reviews
    interval: 5m
    event: review.pending

session:
  max_entries: 20
```

---

## Agent

Identity definition: who this person is, what they know.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Kind | `kind` | string | yes | `"agent"` |
| ID | `id` | string | | Auto-derived from file name |
| Name | `name` | string | | Human-readable name |
| Description | `description` | string | | Role description |
| Skills | `skills` | string[] | | Skill references (see Skill Refs below) |
| Knowhow | `knowhow` | string[] | | Knowhow document references |

### Skill References

Skills can be referenced as:

| Format | Example | Resolution |
|---|---|---|
| Plain name | `code-review` | Local registry lookup |
| Git path | `github.com/org/repo/skill-name` | Fetched from git |
| HTTPS URL | `https://example.com/skill.yaml` | Fetched over HTTP |

### Example

```yaml
kind: agent
name: senior-reviewer
description: Senior engineer focused on code quality and security

skills:
  - code-review
  - security-checklist
  - github.com/roster-community/skills/go-best-practices

knowhow:
  - common-bugs
  - past-incidents
```

---

## Skill

A skill is a YAML file with prompt content.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Name | `name` | string | yes | Skill name |
| Version | `version` | string | yes | Version string |
| Prompt | `prompt` | string | yes | Instruction set handed to the runner |

Knowhow files are plain Markdown (`.md`) placed in a `knowhow/` directory. The file name is the ID.

### Example

```yaml
name: code-review
version: "1.0"
prompt: |
  You are a code reviewer. Focus on:
  - Correctness and edge cases
  - Security vulnerabilities
  - Performance implications
  - Code readability and maintainability
```

---

## Resource

A connection to an external system with watch events and actions.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Kind | `kind` | string | yes | `"resource"` |
| ID | `id` | string | | Auto-derived from file name |
| Name | `name` | string | | Human-readable name |
| Description | `description` | string | | |
| Type | `type` | string | | Resource type (e.g. `"github"`, `"slack"`, `"custom"`) |
| Config | `config` | map[string]string | | Type-specific configuration |
| Watch | `watch` | string[] | | Event types to watch for |
| Actions | `actions` | map[string]*ResourceAction | | Named operations desks can invoke |
| Permissions | `permissions` | PermissionRule[] | | Access control rules. If empty, all actions are open |
| Interval | `interval` | string | | Polling interval for watch (e.g. `"5m"`) |

### ResourceAction

| Field | YAML | Type | Description |
|---|---|---|---|
| Exec | `exec` | string | Script to run |
| Skill | `skill` | string | Skill-backed action (LLM reasoning) |
| Description | `description` | string | Human-readable description |
| Params | `params` | map[string]string | Static parameters |

### PermissionRule

Grants a set of actions to matched desks, groups, or tags. If no permissions are defined, all actions are open to everyone.

| Field | YAML | Type | Description |
|---|---|---|---|
| Allow | `allow` | string[] | Action names to allow. `"*"` means all actions |
| Desks | `desks` | string[] | Desk IDs to match |
| Groups | `groups` | string[] | Group IDs to match |
| Tags | `tags` | string[] | Tag values to match |

### Example

```yaml
kind: resource
name: codebase
description: Main application repository
type: github

config:
  repo: acme/app
  token_env: GITHUB_TOKEN

watch:
  - pull_request
  - issue

interval: 5m

actions:
  commit:
    exec: scripts/commit.sh
    description: Commit changes to the repo
  search:
    skill: search-codebase
    description: Search the codebase using LLM reasoning
  deploy:
    exec: scripts/deploy.sh
    description: Trigger a deployment
    params:
      environment: staging

permissions:
  - allow: [commit, search]
    desks: [implementer-a, implementer-b]
  - allow: [commit, search, deploy]
    groups: [dev-team]
  - allow: [search]
    tags: [viewer]
  - allow: ["*"]
    desks: [admin]
```

---

## Built-in Roster Resource

Every desk automatically has access to a virtual `roster` resource for desk-to-desk communication. No configuration needed.

| Action | Params | Description |
|---|---|---|
| `call` | `desk` (required), `prompt` (required) | Synchronously execute target desk, return its output. Self-calls are rejected |
| `artifact` | `desk` (required) | Return the latest artifact stored for a desk |
| `session` | `desk` (required), `limit` (optional) | Return recent session entries for a desk |

Usage from exec protocol (via stderr):

```
ACTION:{"resource":"roster","action":"call","params":{"desk":"reviewer","prompt":"review this code"}}
ACTION:{"resource":"roster","action":"artifact","params":{"desk":"implementer"}}
ACTION:{"resource":"roster","action":"session","params":{"desk":"architect","limit":"5"}}
```

---

## Policy

Operational rules for retry, timeout, cost, and escalation.

| Field | YAML | Type | Required | Description |
|---|---|---|---|---|
| Kind | `kind` | string | yes | `"policy"` |
| ID | `id` | string | | Auto-derived from file name |
| Name | `name` | string | | Human-readable name |
| Description | `description` | string | | |
| Retry | `retry` | int | | Max retry attempts on failure (default: 0) |
| Timeout | `timeout` | duration | | Max execution time (e.g. `"5m"`, `"1h"`) |
| CostLimit | `cost_limit` | string | | Max cost per invocation (e.g. `"$0.10"`) |
| EscalateTo | `escalate_to` | string | | Desk ID to escalate to on failure |
| OnTimeout | `on_timeout` | string | | `"fail"` (default), `"retry"`, `"escalate"` |
| OnError | `on_error` | string | | `"fail"` (default), `"retry"`, `"escalate"` |
| Budget | `budget` | BudgetConfig | | Multi-granularity cost limits |
| EscalationChain | `escalation_chain` | string[] | | Multi-level escalation: L1 -> L2 -> L3 desk IDs |
| RequireSchema | `require_schema` | string | | Enforce artifact schema (e.g. `"json-v1"`, `"code-v1"`) |

### BudgetConfig

| Field | YAML | Type | Description |
|---|---|---|---|
| Total | `total` | string | Cumulative cost limit (lifetime). Format: `"$500.00"` |
| PerRun | `per_run` | string | Max cost per single run. Format: `"$5.00"` |
| Daily | `daily` | string | Max cost per 24-hour rolling window. Format: `"$50.00"` |
| WarnAt | `warn_at` | float | Fraction (0.0-1.0) at which a warning is emitted. E.g. `0.8` = warn at 80% |

### Example

```yaml
kind: policy
name: careful
description: Conservative policy with budget controls and escalation

retry: 3
timeout: 5m
cost_limit: "$0.10"
on_timeout: escalate
on_error: retry
escalate_to: on-call

budget:
  total: "$500.00"
  per_run: "$5.00"
  daily: "$50.00"
  warn_at: 0.8

escalation_chain:
  - on-call
  - team-lead
  - engineering-manager

require_schema: json-v1
```

---

## Directory Layout

Files can be organized freely; the hub discovers them by `kind` field, not by directory structure. A typical layout:

```
org/
  organization.yaml
  groups/
    strategy-team.yaml
    dev-team.yaml
  desks/
    implementer.yaml
    reviewer.yaml
    architect.yaml
  agents/
    senior-reviewer.yaml
  skills/
    code-review.yaml
  knowhow/
    common-bugs.md
  resources/
    codebase.yaml
    slack-dev.yaml
  policies/
    careful.yaml
  scripts/
    commit.sh
    claude-code.sh
```

## Session Behavior

Sessions are managed automatically, not declared as YAML config files.

| Scope | Lifetime | Content |
|---|---|---|
| Desk session | Persists across runs | Desk's own history and learned context |
| Group session | Resets each run | All desks' outputs within one group run |

Control session loading per desk via the `session.max_entries` field (default: 40, set to 0 to disable).
