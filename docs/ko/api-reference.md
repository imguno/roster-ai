# Roster Hub API Reference

허브 서버가 노출하는 REST API 및 웹훅 엔드포인트입니다.

Base URL: `http://localhost:8080` (기본값)

---

## Pipelines

### 파이프라인 목록 조회

```
GET /api/pipelines
```

**Response:**
```json
["product-launch", "bug-fix", "content-pipeline"]
```

---

### 파이프라인 실행 (수동 트리거)

```
POST /api/trigger/{pipelineID}
```

**Response:**
```json
{"run_id": "a1b2c3d4-..."}
```

---

## Runs

### 실행 이력 조회

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

**상태 값:** `running` | `completed` | `failed` | `skipped`

---

## Events

### 이벤트 로그 조회

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

**이벤트 타입:**

| type | 설명 |
|------|------|
| `desk.started` | 데스크 실행 시작 |
| `desk.completed` | 데스크 완료 |
| `desk.failed` | 데스크 실패 |
| `desk.timed_out` | 데스크 시간 초과 |
| `desk.log` | 데스크 로그 메시지 발생 |
| `human.waiting` | 사람의 입력 대기 중 |
| `human.received` | 사람의 입력 수신 |
| `queue.pushed` | 이벤트가 데스크 큐에 추가됨 |
| `event.published` | 이벤트가 버스에 발행됨 |
| `emit.rejected` | 데스크 허용 목록에 없는 발행 거부됨 |

---

## Human Input (사람 입력)

`human.waiting` 이벤트가 발생한 데스크에 사람의 입력을 제출합니다.

### 입력 제출

```
POST /api/human/{deskID}
Content-Type: application/json

{"content": "입력 내용"}
```

**Response:** `204 No Content`

**Response:** `204 No Content`

---

## Webhooks

외부 서비스에서 파이프라인을 자동으로 시작할 때 사용합니다.

```
POST /webhooks/{pipelineID}
Content-Type: application/json

{"key": "value"}
```

요청 body가 파이프라인 첫 번째 단계의 input payload가 됩니다.

**Response:**
```json
{"run_id": "a1b2c3d4-..."}
```

> **주의:** webhook은 인터넷에서 접근 가능한 서버에서만 동작합니다.
> 로컬 머신에서는 `exec` 또는 `polling` 트리거를 사용하세요.

---

## gRPC Worker Protocol

Remote worker와의 통신 프로토콜입니다.

**Proto 파일:** `roster/proto/worker.proto`

```
service Worker {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

워커 시작:
```bash
roster worker :50051
```

데스크 설정:
```yaml
executor:
  type: remote
  params:
    address: "worker-host:50051"
```
