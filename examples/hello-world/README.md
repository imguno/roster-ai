# Hello World

A minimal Roster organization that develops a Python library.

The team builds "Taskflow" — a lightweight async task queue. A developer writes the code, a reviewer checks and improves it.

## Structure

```
organization.yaml         → routes task.created to dev-team
groups/dev-team.yaml       → developer writes, reviewer finalizes
desks/developer.yaml       → Claude Sonnet via API
desks/reviewer.yaml        → Claude Sonnet via API
agents/developer.yaml      → developer identity + skills
agents/reviewer.yaml       → reviewer identity + skills
skills/develop.md          → project requirements + architecture
skills/review.md           → review standards + checklist
```

## Run

```bash
export ANTHROPIC_API_KEY=sk-ant-...
cd examples/hello-world

# 1. Validate — check for config errors before running
roster dry-run .

# 2. Start the hub with the web dashboard
roster hub --ui :8080

# 3. Send a task (in another terminal)
roster emit task.created "Implement the core TaskQueue and @task decorator"
```

## Using the Dashboard

Open **http://localhost:8080** after starting the hub.

### Graph View (default)

The main screen shows your organization as a live graph. You'll see:

- **Nodes** for each desk (`developer`, `reviewer`) and group (`dev-team`)
- **Edges** showing how events flow between them
- **Live status** — nodes light up as desks start working

Click any node to open the **Detail Panel** on the right, which shows:
- Current status (idle / running / done)
- Queue depth
- Token usage and cost

### Runs View

Click **▤ Runs** in the top bar to see execution history.

- Each run shows: timestamp, status (completed/failed), duration
- Click a run to drill down into **per-step details** — which desk ran, how long it took, what it produced
- Expand a step to see the actual output (the code the developer wrote, the reviewer's feedback)

### Routing View

Click **↔ Routing** to see the event routing table:

- `task.created → dev-team` — your routing rule visualized
- Shows subscribe/emit connections between all desks and groups

### Top Bar Stats

The top bar shows live counters:

- **Desks** — number of active desks
- **Groups** — number of groups
- **Events** — total events processed
- **Cost** — cumulative API cost across all desks

### Try It

1. Watch the **Graph View** while a run executes — see `developer` light up, then `reviewer`
2. Switch to **Runs** after completion — click the run to read the generated code
3. Send another task: `roster emit task.created "Add error handling and retry logic to TaskQueue"`
4. Compare the two runs side by side in the Runs view
