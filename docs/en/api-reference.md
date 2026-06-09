# Roster Hub API Reference

REST API and webhook endpoints exposed by the hub server.

Base URL: `http://localhost:8080` (default)

---

## Pipelines

### List Pipelines

```
GET /api/pipelines
```

**Response:**
```json
["product-launch", "bug-fix", "content-pipeline"]
```

---

### Trigger a Pipeline (manual trigger)

```
POST /api/trigger/{pipelineID}
```

**Response:**
```json
{"run_id": "a1b2c3d4-..."}
```

---

## Runs

### Get Run History

```
GET /api/runs
```

**Response:**
```json
[
  {
    "ID": "a1b2c3d4-...",
    "PipelineID": "product-launch",
    "Status": "done",
    "StartedAt": "2026-06-07T10:00:00Z",
    "FinishedAt": "2026-06-07T10:05:30Z",
    "Error": ""
  }
]
```

**Status values:** `pending` | `running` | `done` | `failed` | `waiting`

---

## Events

### Get Event Log

```
GET /api/events
GET /api/events?pipeline={pipelineID}
```

**Response:**
```json
[
  {
    "PipelineID": "product-launch",
    "StepID": "writer-desk",
    "Type": "step.completed",
    "At": "2026-06-07T10:01:00Z",
    "DurationMs": 3200,
    "Model": "claude-opus-4-8",
    "InputBytes": 1200,
    "OutputBytes": 800,
    "Error": ""
  }
]
```

**Event types:**

| type | description |
|------|-------------|
| `step.started` | a step has begun executing |
| `step.completed` | a step completed successfully |
| `step.failed` | a step failed |
| `gate.waiting` | waiting for approval |
| `gate.approved` | approved |
| `human.waiting` | waiting for human input |
| `human.received` | human input received |

---

## Gates (Approval Gates)

### List Pending Gates

```
GET /api/gates
```

**Response:**
```json
[
  {
    "RunID": "a1b2c3d4-...",
    "StepID": "prd-review",
    "At": "2026-06-07T10:02:00Z"
  }
]
```

---

### Approve / Reject a Gate

```
POST /api/gates/{runID}/{stepID}/approve
POST /api/gates/{runID}/{stepID}/reject
```

- `approve` → proceed to the next step
- `reject` → roll back to the previous work step and re-execute

**Response:** `204 No Content`

---

## Human Input (Human-in-the-Loop Steps)

Receives human input for pipeline steps that have an `executor: type: human` desk.

### Submit Input

```
POST /api/human/{runID}/{stepID}
Content-Type: application/json

{"content": "Enter your content here directly"}
```

- The submitted `content` becomes the output artifact for that step
- The next step receives this artifact as its input

**Response:** `204 No Content`

---

## Webhooks

Used to automatically trigger a pipeline from an external service.

```
POST /webhooks/{pipelineID}
Content-Type: application/json

{"key": "value"}
```

The request body becomes the input payload for the first step of the pipeline.

**Response:**
```json
{"run_id": "a1b2c3d4-..."}
```

> **Note:** Webhooks only work when the server is accessible from the internet.
> For local machines, use an `exec` or `polling` trigger instead.

---

## gRPC Worker Protocol

The communication protocol used with remote workers.

**Proto file:** `roster/proto/worker.proto`

```
service Worker {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

Starting a worker:
```bash
roster worker :50051
```

Desk configuration:
```yaml
executor:
  type: remote
  params:
    address: "worker-host:50051"
```
