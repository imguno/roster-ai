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
    "ID": "a1b2c3d4-...",
    "PipelineID": "product-launch",
    "Status": "done",
    "StartedAt": "2026-06-07T10:00:00Z",
    "FinishedAt": "2026-06-07T10:05:30Z",
    "Error": ""
  }
]
```

**Status 값:** `pending` | `running` | `done` | `failed` | `waiting`

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

**이벤트 타입:**

| type | 설명 |
|------|------|
| `step.started` | 단계 실행 시작 |
| `step.completed` | 단계 완료 |
| `step.failed` | 단계 실패 |
| `gate.waiting` | 승인 대기 중 |
| `gate.approved` | 승인됨 |
| `human.waiting` | 사람의 입력 대기 중 |
| `human.received` | 사람의 입력 수신 |

---

## Gates (승인 게이트)

### 대기 중인 게이트 목록

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

### 게이트 승인 / 거절

```
POST /api/gates/{runID}/{stepID}/approve
POST /api/gates/{runID}/{stepID}/reject
```

- `approve` → 다음 단계로 진행
- `reject` → 직전 작업 단계로 롤백 후 재실행

**Response:** `204 No Content`

---

## Human Input (사람 참여 단계)

`executor: type: human` 데스크가 있는 파이프라인 스텝에서 사람의 입력을 받습니다.

### 입력 제출

```
POST /api/human/{runID}/{stepID}
Content-Type: application/json

{"content": "여기에 직접 작성한 내용을 입력합니다"}
```

- 입력된 `content`가 해당 단계의 아웃풋 아티팩트가 됩니다
- 다음 단계는 이 아티팩트를 input으로 받습니다

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
