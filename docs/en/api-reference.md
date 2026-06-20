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
    "run_id": "a1b2c3d4-...",
    "group_id": "product-launch",
    "desks": ["writer-desk", "reviewer-desk"],
    "status": "completed",
    "started_at": "2026-06-07T10:00:00Z",
    "total_step_ms": 5300,
    "input_tokens": 1200,
    "output_tokens": 800,
    "trigger_type": "product.draft.ready"
  }
]
```

**Status values:** `running` | `completed` | `failed` | `skipped`

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
    "RunID": "a1b2c3d4-...",
    "DeskID": "writer-desk",
    "Type": "desk.completed",
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
| `desk.started` | a desk has begun executing |
| `desk.completed` | a desk completed successfully |
| `desk.failed` | a desk failed |
| `desk.timed_out` | a desk timed out |
| `desk.log` | a desk emitted a log message |
| `human.waiting` | waiting for human input |
| `human.received` | human input received |
| `queue.pushed` | event queued for a desk |
| `event.published` | event published to bus |
| `emit.rejected` | emission not in desk's allow-list |

---

## Human Input

Submit input for desks waiting for human input (`human.waiting` event).

### Submit Input

```
POST /api/human/{deskID}
Content-Type: application/json

{"content": "Your input here"}
```

**Response:** `204 No Content`

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
