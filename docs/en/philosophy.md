# Roster Philosophy

## One Principle: Organization as Code

Infrastructure as Code provisions servers. Roster provisions organizations.

You declare desks, groups, resources, and routing in YAML.
Roster stands up a team that works — always running, always ready.

---

## You Are the CEO

Roster models real-world organizations, not software abstractions.

- You **hire agents** — define who they are, what they know (skills), what they've learned (knowhow)
- You **assign desks** — connect agents to executors (LLM, script, human, remote)
- You **form teams** — groups of desks that share context and collaborate
- You **set up routing** — events flow between teams, just like work flows between departments
- You **observe and intervene** — dashboard shows what's happening, you emit events to steer

You do not think about DAGs, polling intervals, or message queues.
That is Roster's job.

---

## Everything Is an Event

There is exactly one communication pattern: **events**.

- Teams emit events when work is done
- Other teams subscribe to those events
- Resources emit events when the world changes (file modified, PR opened, timer fired)
- Routing rules connect producers to consumers

```
design-team emits design.approved
  → routing delivers to dev-team
  → dev-team implements, emits code.ready
  → routing delivers to qa-team
  → qa-team tests, emits qa.passed
  → routing delivers to deploy-team
```

No special syntax for inter-team vs intra-team communication.
No markers in output text. No custom protocols between components.
Events in, events out. That's it.

---

## Resources Are Shared State

Resources are the observable layer. They connect the organization to the outside world
and to each other.

- A **task-board** resource is a directory. Writing a file triggers the target team.
  Open the directory — you see every task ever assigned.
- A **codebase** resource wraps git. Desks can commit, read files, run scripts.
- A **slack** resource forwards messages. No LLM needed — just a script.

Resources have **actions** (write, commit, notify) and **watchers** (file changes, polling).
Watchers emit events. Actions are invoked by desks via the exec protocol.

The key insight: resources make communication **observable and persistent**.
Events are ephemeral. Resource state is durable. Teams communicate through both.

---

## Groups Are Teams, Not Pipelines

A group is a team of desks that share context. Not a sequence of steps.

```yaml
kind: group
name: dev-team
lead:
  desk: implementer
groups:
  - impl-squad      # nested group: implementation
  - review-squad    # nested group: review
```

The lead coordinates: plans first, synthesizes last. Members work in between.
Members see each other's output through the **group session** — shared context
that accumulates within a run.

**Conversation mode** (`dispatch: conversation`): members run multiple rounds,
responding to each other. Not a fixed pipeline — an organic discussion where
each member can build on, disagree with, or extend what others said.

Members can **SKIP** when they have nothing to add. This is not failure — it's
self-governance. Over time, each desk learns when its contribution matters.

---

## LLM Where Judgment Is Needed, Scripts Where It Isn't

Not everything needs an LLM.

| Needs LLM (judgment, creativity) | Script is enough (deterministic) |
|---|---|
| Design review | Build / compile |
| Code implementation | Test suite execution |
| Content writing | File conversion |
| Priority decisions | Notification forwarding |
| Data analysis | Scheduled backup |

An LLM call costs time and money. A shell script is instant and free.
Use the right tool for the job.

---

## Knowhow Accumulates

Skills are what you hire an agent with — pre-defined knowledge.
Knowhow is what the agent learns on the job.

```
skills/                 # authored by humans, versioned in git
  product-strategy.md
  code-review.md

knowhow/                # extracted from work, accumulated over time
  common-pitfalls.md
  customer-patterns.md
```

When a desk produces output containing `## Knowhow`, the hub extracts it
and saves it to the knowhow directory. Next time that agent runs,
the learned patterns are included in its prompt.

Skills compose into agents. Knowhow accumulates as the system learns.

---

## Self-Governance Over Central Control

Roster prefers autonomous teams over micromanaged pipelines.

- **SKIP**: A desk that has nothing to add says so and steps aside.
  No wasted LLM calls, no forced output.
- **Conversation**: Members in a group discuss, not just execute sequentially.
  Two implementers can split work organically instead of waiting for a dispatcher.
The goal: define the organization once, and it runs itself.
Intervene when you want to steer, not because the system can't function without you.

---

## Artifacts Are the Work Product

Every desk produces an artifact. Every artifact is saved as a file.

```
.roster/data/artifacts/
  design-team-designer-20260608-1000-a3b4c5d.md
  dev-team-implementer-20260608-1100-f6g7h8i.md
  qa-team-tester-20260608-1200-j1k2l3m.md
```

Artifacts are how teams communicate. The design team's mockup becomes the dev team's spec.
The dev team's code becomes the QA team's test target.
The same artifact goes to Slack, to the dashboard, to the next team in the chain.

This replaces hidden internal state with **visible work product**.
Open the artifacts directory — you see every decision, every output, every handoff.

---

## Design Principles

1. **Organization, not orchestration** — model how teams work, not how software schedules tasks
2. **One communication pattern** — events and resources, everywhere, for everything
3. **Observable by default** — open a file, see what happened; open the dashboard, see what's running
4. **Durable state** — queues persist, sessions persist, knowhow persists; crashes are recoverable
5. **Minimal intervention** — teams self-govern; humans steer, not operate
6. **Right tool for the job** — LLM for judgment, scripts for determinism
7. **Composable** — desks, groups, skills, and resources are reusable building blocks
8. **Artifacts over messages** — work product is saved as files, not lost in event streams
