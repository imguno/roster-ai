# Roster — Core Concepts

Roster is **Organization as Code**.

Like Infrastructure as Code provisions servers, Roster provisions organizations — desks, groups, resources, routing — declared in YAML, versioned in git, reproducible anywhere.

You're not writing a script that runs AI. You're standing up a team that works.

Everything in Roster is built from nine small concepts. They compose.

---

## Event

An **Event** is the universal message. Everything that flows through the system is an event — a new task arriving, a desk finishing its work, a resource changing, a person responding.

Desks subscribe to events. Desks emit events. That's the whole communication model.

---

## Desk

A **Desk** is an independent actor. It receives events, does one job, and emits events.

```yaml
kind: desk
name: reviewer
executor:
  type: api          # how it runs
agent: senior-reviewer
```

Executor types:
- **api** — LLM call (Anthropic, OpenAI, Gemini)
- **exec** — any process (script, CLI tool, Python)
- **docker** — containerized process
- **human** — sends a notification, waits for a person to respond
- **remote** — worker on another machine

A human is just a desk with `type: human`. No special concept needed.

---

## Agent

An **Agent** is a reusable role. It defines *who* sits at a desk — what they know, how they think.

```yaml
kind: agent
name: senior-reviewer
skills:
  - code-review
  - security-checklist
```

Define once, use across many desks. Or define inline if the role is only used in one place.

---

## Skill

A **Skill** is a reusable knowledge fragment — a prompt, a guideline, a learned pattern.

```
skills/
  code-review.md        # how to review code
  security-checklist.md # what to look for

knowhow/
  common-bugs.md        # patterns accumulated over time
```

Skills compose into agents. Knowhow accumulates as the system learns.

---

## Group

A **Group** is a team. Desks in a group share an event stream — every desk sees what the others produce.

```yaml
kind: group
name: dev-team
lead: implementer      # coordinates: plans first, synthesizes last

desks:
  - implementer-b
  - reviewer
  - tester
```

The lead desk runs twice: first to plan and assign work, last to synthesize results. Members work in between.

```
lead (plan)  →  members (work)  →  lead (synthesize)  →  output event
```

---

## Resource

A **Resource** is a connection to an external system. It has two sides:

**Watch** — emits events when something changes (GitHub PR opened, Figma updated, message received).

**Actions** — desks can request operations, backed by scripts or skills.

```yaml
kind: resource
name: codebase
type: github
repo: acme/app

watch:
  - pull_request
  - issue

actions:
  commit:
    exec: scripts/commit.sh
  notify:
    exec: scripts/slack.sh

permissions:
  patcher:   [commit]
  reporter:  [notify]
```

Resources can be fully custom — define your own events and actions backed by any script. Built-in types (github, figma, slack) are just pre-packaged resources.

---

## Organization

An **Organization** is the whole system — always running, always ready.

It defines which groups exist, which resources they connect to, and how events route between them.

```yaml
kind: organization
name: engineering

groups:
  - strategy-team
  - dev-team
  - ops-team

routing:
  - on: task.created   → strategy-team
  - on: plan.ready     → dev-team
  - on: code.ready     → ops-team
```

Work comes in as an event. The organization routes it to the right group. The group handles it. No manual wiring per task.

---

## Session

A **Session** is memory.

**Desk session** — each desk's own history. Persists across runs. The reviewer remembers what it flagged last time.

**Group session** — shared context within a group run. All desks see each other's work. Resets each run.

---

## Policy

A **Policy** is a set of rules attached to a desk or group.

```yaml
kind: policy
name: careful
retry: 3
timeout: 5m
cost_limit: $0.10
```

Retry on failure, time out if stuck, cap spending. Attach to any desk or group.

---

## How They Compose

```
Skill + Skill           → Agent
Agent + Executor        → Desk
Desk + Desk + Resource  → Group
Group + Group + Routing → Organization
```

A small team:

```
resource: github-repo (watch: pull_request)
      ↓ event: pr.opened
organization routes → dev-team
      ↓
lead desk plans → members review + test → lead synthesizes
      ↓ event: review.done
resource action: github-repo.notify
```

That's the whole model.

---

## Current State

| Concept | Status |
|---|---|
| Event (universal message, event bus) | ✅ |
| Desk (api, exec, docker, human, remote) | ✅ |
| Agent + Skill | ✅ |
| Group with coordinator pattern (both/first/last) | ✅ |
| Resource (type definition + watch placeholder) | ✅ |
| Organization (event routing between groups/desks) | ✅ |
| Session (desk + group) | ✅ |
| Policy (type definition) | ✅ |
| Actor model event bus | ✅ |
| Resource watchers (real polling/webhook) | 🔲 |
| Policy enforcement (retry, timeout, cost) | 🔲 |
