# Roster YAML Reference

All configuration files declare their type using the `kind:` field.
Files can live anywhere — the hub will find them automatically.

---

## kind: agent

An agent's identity. Defines what skills it has.

```yaml
kind: agent
id: researcher           # unique ID (required)
name: Researcher
skills:
  - web-search           # looked up in the local skills/ folder
  - summarize            # skills/summarize.yaml
  - github.com/org/roster-skills/translate-v1   # community skill
  - https://skills.roster.io/code-review-v1     # URL skill
```

Inline agent declaration: inside a desk file, you can use an object for the `agent:` field instead of a string (ID reference) to define the agent inline alongside the desk.

---

## kind: desk

An agent's execution environment. Receives events, performs tasks, and emits events.

```yaml
kind: desk
id: researcher-desk      # unique ID (required)
agent: researcher        # agent ID or relative path (./path/agent.yaml)

executor:
  type: api              # api | exec | docker | remote | human
  sdk: anthropic         # api only: anthropic | openai | gemini
  params:
    model: claude-sonnet-4-6
    command: "python script.py"   # exec only
    image: "my-org/agent:latest"  # docker only
    address: "host:50051"         # remote only
  env:
    api_key: "${ANTHROPIC_API_KEY}"

concurrency:
  mode: queue            # queue | spawn | reject
  max: 3                 # spawn only: maximum number of concurrent executions

subscribe:               # list of event types this desk will receive
  - plan.approved
  - code.ready

emit:                    # event types to emit when work is complete
  - research.done

# TIP: Events listed in `emit` are fired automatically when the desk's
# executor finishes (script exits, API call returns, process completes).
# Prefer this deterministic approach over having the LLM decide to emit
# during generation — it keeps your event flow predictable and reproducible.

cron: "0 */6 * * *"     # optional: run automatically on a cron schedule (every 6 hours)

resources:               # resources accessible only to this desk
  - github-api

tags:                    # tags for role-based permission matching
  - backend
  - senior

policy: careful          # optional: policy reference (retry/timeout/cost)
```

### Executor Types

| type | description | required params |
|------|-------------|-----------------|
| `api` | built-in AI SDK | `sdk`, `model` |
| `exec` | arbitrary command (stdin/stdout JSON) | `command` |
| `docker` | Docker container | `image` |
| `remote` | gRPC remote worker | `address` |
| `human` | a person performs the task directly via the web UI | none |

### Concurrency Modes

| mode | behavior |
|------|----------|
| `queue` | process requests one at a time, in order (default) |
| `spawn` | run up to `max` instances concurrently |
| `reject` | reject the request if already running |

---

## kind: group

A team. A collection of desks. Activated when it receives an event; member desks collaborate using a shared context (group session).

```yaml
kind: group
id: dev-team             # unique ID (required)
name: Dev Team

lead:
  desk: architect        # the lead desk that coordinates the group
  position: both         # both (default) | first | last

desks:
  - frontend-desk
  - backend-desk

groups:                  # nested groups (optional)
  - design-team

resources:               # resources shared across the entire group
  - codebase
  - backlog

subscribe:               # event types this group will receive
  - plan.approved

emit:                    # event types to emit when work is complete
  - code.ready           # (see Desk emit TIP above — same principle applies)

cron: "0 */3 * * *"     # optional: run automatically on a cron schedule

policy: standard         # optional: policy reference

dispatch: sequential     # sequential (default) | parallel | conversation
```

### Lead Position

| position | execution order |
|----------|----------------|
| `both` (default) | lead → members → lead (plan/assign → work → synthesize) |
| `first` | lead → members (lead breaks down the work and passes it to members) |
| `last` | members → lead (lead aggregates the results from members) |

### Dispatch Modes

| dispatch | behavior |
|----------|----------|
| `sequential` (default) | execute members in order; each result feeds into the next as input |
| `parallel` | send the same input to all members → run concurrently → aggregate results (not yet implemented, planned) |
| `conversation` | multi-round sequential execution; members converse with each other |

---

## kind: organization

The top-level system definition. Declares group composition and event routing.
Declare only one per project.

```yaml
kind: organization
id: engineering          # unique ID (required)
name: Engineering Org

groups:
  - strategy-team
  - dev-team
  - ops-team

resources:               # resources shared across the entire organization
  - codebase
  - slack

routing:
  - on: plan.approved    # when this event is received
    to: dev-team         # forward to this group (or desk)
  - on: code.ready
    to: ops-team
  - on: hub.started      # event published automatically when the hub starts
    to: strategy-team
```

Routing rules can also be declared at the group/desk level using the `subscribe` field, in addition to the organization level.
Both approaches behave identically.

---

## kind: resource

A connection to an external system. Any external state — files, git, APIs, etc. — can be declared as a resource.

```yaml
kind: resource
id: codebase             # unique ID (required)
name: Codebase
type: file               # resource type (file, github, user-defined, etc.)

config:
  root: .                # resource root path
  path: roster/          # path to operate on

watch:                   # event types to emit when a change is detected
  - file.changed

interval: "5m"           # watch polling interval (default 60s)

actions:
  read:
    exec: scripts/read-files.sh
    description: Read file contents
  commit:
    exec: scripts/git-commit.sh
    description: Commit changes to git

permissions:
  - allow: ["*"]         # allow all actions
    groups: [dev-team, ops-team]
  - allow: [read]
    tags: [viewer]
```

For a desk to invoke a resource action, the exec runner uses the `ACTION:` protocol over stderr.
See the exec protocol section of the developer guide for details.

### Permission Rules

If no permissions are defined, all desks/groups can access all actions.
If permissions are defined, only desks/groups that match a rule can access.

| field | description |
|-------|-------------|
| `allow` | list of actions to allow; `"*"` allows all |
| `desks` | list of desk IDs |
| `groups` | list of group IDs |
| `tags` | list of desk tags |

---

## kind: policy

Operational rules applied to a desk or group. Configures retry, timeout, and cost limits.

```yaml
kind: policy
id: careful              # unique ID (required)
name: Careful Policy

retry: 3                 # maximum number of retries on failure
timeout: 5m              # maximum execution time (e.g. 30s, 5m, 1h)
cost_limit: "$0.10"      # maximum cost limit per invocation
```

Applying to a desk or group:
```yaml
kind: desk
id: expensive-desk
policy: careful
```

---

## kind: skill

A prompt package used by an agent.

```yaml
kind: skill
id: web-search-v1
name: Web Search
version: "1"
description: Search the web for relevant information

prompt: |
  You are a research assistant. When given a topic or question,
  search for the most relevant and recent information.

  Format your response as:
  1. Key findings
  2. Sources
  3. Summary
```

Markdown (`.md`) or text (`.txt`) files can also be used as skills — without a `prompt:` field, the entire file contents are used as the prompt.

---

## kind: pipeline

A sequential execution pipeline. Runs nodes in an explicit order.

> **Note**: pipeline is a legacy model. For new projects, the organization + subscribe/emit event-driven model is recommended.

```yaml
kind: pipeline
id: feature-launch       # unique ID (required)

trigger:                 # automatic trigger (omit for manual execution)
  type: exec             # exec | polling | webhook | manual
  command: "./check.py"
  interval: "5m"

steps:
  - desk: me
    label: requirements

  - agent: researcher
    trigger:
      from: [requirements]

  - group: dev-team
    trigger:
      from: [researcher]
    gate: approve        # approve or reject via the web UI

  - desk: deployer-desk
    trigger:
      from: [dev-team]
```

---

## Environment Variable Substitution

Environment variables can be referenced anywhere in YAML values using the `${VAR_NAME}` syntax.

```yaml
env:
  api_key: "${ANTHROPIC_API_KEY}"
  repo: "${GITHUB_REPO}"
```

---

## Example Project Structure

Files can live anywhere. As long as a file has a `kind:` field, the hub will find it automatically.

```
my-org/
├── organization.yaml     # kind: organization
├── agents/
│   └── architect.yaml    # kind: agent (or defined inline inside desks/)
├── desks/
│   ├── architect.yaml    # kind: desk
│   └── implementer.yaml
├── groups/
│   ├── strategy-team.yaml  # kind: group
│   └── dev-team.yaml
├── resources/
│   ├── codebase.yaml     # kind: resource
│   └── slack.yaml
├── policies/
│   └── standard.yaml     # kind: policy
└── skills/
    └── code-review.md    # skill prompt
```
