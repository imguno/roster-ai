# Quickstart: Your First AI Agent Pipeline

Get from zero to a working AI pipeline in 10 minutes.

---

## Prerequisites

- Go 1.22+
- An Anthropic API key ([get one here](https://console.anthropic.com))

---

## 1. Install Roster

```bash
git clone https://github.com/roster-io/roster
cd roster/roster
go build -o roster ./cmd/roster
# Optionally move to your PATH
mv roster /usr/local/bin/roster
```

---

## 2. Set your API key

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

---

## 3. Scaffold a new organization

```bash
roster init my-org
cd my-org
```

This creates:

```
my-org/
├── organization.yaml   # routing rules
├── agents/
│   └── developer.yaml  # agent identity + skills
├── desks/
│   └── developer.yaml  # how the agent runs
├── groups/
│   └── dev-team.yaml   # team of desks
├── skills/
│   └── coding.md       # prompt skills
└── policies/
    └── standard.yaml   # retry + timeout
```

---

## 4. Verify the executor config

`roster init` already generates a working API executor in `desks/developer.yaml`:

```yaml
kind: desk
name: developer

agent: developer

executor:
  type: api
  sdk: anthropic
  params:
    model: claude-haiku-4-5-20251001
    api_key: ${ANTHROPIC_API_KEY}

policy: standard
```

The `${ANTHROPIC_API_KEY}` placeholder is expanded from your environment at load time. No manual edits needed.

---

## 5. Validate the config

```bash
roster dry-run .
```

Expected output:
```
  ✓ Config loaded: 1 desks, 1 groups, 0 resources, 1 policies
  ✓ Validation passed
  ✓ All skills resolved
  ✓ Dry-run complete
```

---

## 6. Start the hub

```bash
roster hub --ui :8080
```

The hub starts and prints the dashboard URL. Open `http://localhost:8080` in your browser.

---

## 7. Send your first task

In a second terminal:

```bash
roster emit task.created "Write a Go function that checks if a number is prime"
```

The event flows through the routing rules:

```
task.created  →  dev-team (group)  →  developer (desk)  →  Claude API
```

---

## 8. See the result

**Dashboard**: Open `http://localhost:8080` — the Event Log shows token counts, timing, and model used.

**CLI**:
```bash
roster logs --follow
```

---

## What just happened?

The `organization.yaml` defines a routing rule:

```yaml
kind: organization
name: my-org

groups:
  - dev-team

routing:
  - on: task.created
    to: dev-team
```

The `groups/dev-team.yaml` subscribes to `task.created` and routes work to the `developer` desk:

```yaml
kind: group
name: dev-team

desks:
  - developer

subscribe:
  - task.created

emit:
  - task.completed
```

When you emitted `task.created`, the hub matched the routing rule, activated the group, and the developer desk called Claude with your coding skill prompt + your task description.

---

## Next steps

- **Add more desks** — reviewers, testers, deployers
- **Form a team** — put multiple desks in one group, add a lead desk to coordinate
- **Connect tools** — define a `resource` to give desks access to GitHub, Slack, or any API
- **Schedule work** — add `cron: "0 9 * * *"` to a desk or group to run on a schedule
- **Human in the loop** — use `executor: {type: human}` for approval gates

See the [YAML reference](yaml-schema.md) for all configuration options.

---

## Templates

`roster init` includes three additional templates for common use cases:

```bash
roster init my-org --template product-team     # architect + dev + review + ops
roster init my-org --template content-pipeline # researcher + writer + editor
roster init my-org --template code-review      # security + quality reviewers (parallel)
```
