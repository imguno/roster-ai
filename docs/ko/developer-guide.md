# Roster Developer Guide

일반 사용자는 YAML만으로 Roster를 사용합니다.
개발자는 커스텀 executor, trigger, adapter를 만들어 Roster 위에서 돌릴 수 있습니다.

---

## 확장 방식 세 가지

| 방식 | 대상 | 언어 | 설명 |
|------|------|------|------|
| **Exec Protocol** | Executor | 모든 언어 | stdin JSON → stdout JSON |
| **gRPC Worker** | Executor | 모든 언어 | Roster worker 서비스 구현 |
| **Go SDK** | Executor/Trigger/Adapter | Go | `sdk.Executor`, `sdk.Trigger` 인터페이스 구현 |

---

## 1. Exec Protocol — 가장 쉬운 방법

어떤 언어로도 Roster executor를 만들 수 있습니다.

### 데스크 설정

```yaml
kind: desk
id: my-custom-desk
agent: my-agent
executor:
  type: exec
  params:
    command: "python my_executor.py"
  env:
    MY_API_KEY: "${MY_API_KEY}"
```

### stdin 형식 (허브 → 실행기)

```json
{
  "agent_id": "researcher",
  "desk_id": "researcher-desk",
  "prompt": "<merged skill prompts + input context>",
  "session": [
    {"role": "user", "content": "이전 작업 컨텍스트"},
    {"role": "assistant", "content": "이전 응답"}
  ],
  "group_history": [
    {"desk_id": "designer-desk", "role": "assistant", "content": "디자인 완성했습니다"}
  ],
  "input": "<이전 단계 아웃풋 텍스트, 없으면 빈 문자열>"
}
```

### stdout 형식 (실행기 → 허브)

```json
{
  "schema": "text-v1",
  "payload": "실행 결과 텍스트"
}
```

- `schema`: 아티팩트 타입 식별자 (자유롭게 정의. 예: `"text-v1"`, `"code-v1"`, `"json-v1"`)
- `payload`: 다음 단계에 전달될 아웃풋

**raw stdout fallback**: JSON이 아니면 전체 stdout을 `text-v1` payload로 처리합니다.

### 예시: Python executor

```python
#!/usr/bin/env python3
import sys, json

data = json.load(sys.stdin)

prompt = data["prompt"]
session = data.get("session", [])
group_history = data.get("group_history", [])

# 여기에 어떤 AI/로직이든 넣으면 됩니다
result = call_my_llm(prompt, session)

print(json.dumps({"schema": "text-v1", "payload": result}))
```

### 예시: Node.js executor

```javascript
const chunks = [];
process.stdin.on('data', c => chunks.push(c));
process.stdin.on('end', async () => {
  const data = JSON.parse(Buffer.concat(chunks).toString());
  const result = await callMyApi(data.prompt);
  console.log(JSON.stringify({ schema: 'text-v1', payload: result }));
});
```

---

## 2. gRPC Worker — 원격 실행

다른 서버(GPU 서버, 보안망 내부 서버 등)에서 에이전트를 실행할 때 사용합니다.

### Worker 서버 실행

```bash
roster worker :50051
```

### 데스크 설정

```yaml
kind: desk
id: gpu-agent-desk
agent: my-agent
executor:
  type: remote
  params:
    address: "gpu-server.internal:50051"
```

### 커스텀 Worker 서버 구현

`proto/worker.proto`를 사용해 어떤 언어로도 구현할 수 있습니다.

```protobuf
service Worker {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

**Python 예시 (grpc):**

```python
import grpc
from concurrent import futures
import worker_pb2, worker_pb2_grpc

class WorkerServicer(worker_pb2_grpc.WorkerServicer):
    def Execute(self, request, context):
        result = my_model.generate(request.prompt)
        return worker_pb2.ExecuteResponse(
            schema="text-v1",
            payload=result.encode()
        )

server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
worker_pb2_grpc.add_WorkerServicer_to_server(WorkerServicer(), server)
server.add_insecure_port('[::]:50051')
server.start()
server.wait_for_termination()
```

---

## 3. Go SDK — 가장 강력한 방법

Go로 작성하면 Roster를 라이브러리로 임포트해 완전히 커스텀한 허브를 만들 수 있습니다.

### 커스텀 Executor

```go
import "github.com/roster-io/roster/pkg/sdk"

type MyExecutor struct{}

func (e *MyExecutor) Run(ctx context.Context, task sdk.Task) (*types.Artifact, error) {
    // task.Prompt        — 실행할 내용
    // task.Session       — 데스크의 이전 대화 기록
    // task.GroupHistory  — 팀 공유 공간의 메시지들
    // task.Options       — 데스크 YAML의 params
    // task.Env           — 환경 변수

    result := callMyService(task.Prompt)
    return &types.Artifact{
        Schema:  "text-v1",
        Payload: []byte(result),
    }, nil
}
```

등록 방법 (main.go에서):

```go
registry := runner.NewRegistry()
registry.Register("my-custom", &MyExecutor{})
```

데스크 YAML에서 사용:

```yaml
executor:
  type: my-custom
  params:
    endpoint: "https://my-service.com/api"
```

### 커스텀 Trigger

```go
import "github.com/roster-io/roster/pkg/sdk"

type SlackTrigger struct {
    channel string
    token   string
}

func (t *SlackTrigger) Start(ctx context.Context) (<-chan sdk.TriggerEvent, error) {
    ch := make(chan sdk.TriggerEvent, 4)
    go func() {
        defer close(ch)
        for {
            select {
            case <-ctx.Done():
                return
            case msg := <-t.pollSlack(ctx):
                ch <- sdk.TriggerEvent{
                    PipelineID: "handle-slack-message",
                    Payload:    map[string]string{"text": msg},
                }
            }
        }
    }()
    return ch, nil
}
```

### 커스텀 Channel Adapter

```go
import "github.com/roster-io/roster/pkg/sdk"

type NotionAdapter struct {
    pageID string
}

func (a *NotionAdapter) Send(ctx context.Context, artifact *types.Artifact) error {
    return notion.UpdatePage(ctx, a.pageID, string(artifact.Payload))
}

func (a *NotionAdapter) Receive(ctx context.Context) (*types.Artifact, error) {
    content, err := notion.GetPage(ctx, a.pageID)
    if err != nil {
        return nil, err
    }
    return &types.Artifact{Schema: "text-v1", Payload: []byte(content)}, nil
}
```

---

## SDK 인터페이스 레퍼런스

### `sdk.Executor`

```go
type Executor interface {
    Run(ctx context.Context, task Task) (*types.Artifact, error)
}
```

### `sdk.Task`

```go
type Task struct {
    AgentID      string
    DeskID       string
    Prompt       string            // 스킬 프롬프트 + 입력 컨텍스트
    Input        *types.Artifact   // 이전 단계 아웃풋 (nil = 첫 단계)
    Options      map[string]string // 데스크 YAML의 executor.params
    Env          map[string]string // 환경 변수
    Session      []SessionEntry    // 데스크 영구 세션 기록
    GroupHistory []GroupMessage    // 팀 공유 소통 기록
}
```

### `sdk.Trigger`

```go
type Trigger interface {
    Start(ctx context.Context) (<-chan TriggerEvent, error)
}

type TriggerEvent struct {
    PipelineID string
    Payload    map[string]string
}
```

### `sdk.Adapter`

```go
type Adapter interface {
    Send(ctx context.Context, artifact *types.Artifact) error
    Receive(ctx context.Context) (*types.Artifact, error)
}
```

---

## 아티팩트 Schema 컨벤션

커스텀 schema 이름은 자유롭게 정하되, 표준 이름을 권장합니다:

| schema | payload 형식 |
|--------|-------------|
| `text-v1` | UTF-8 텍스트 |
| `json-v1` | JSON 객체 |
| `code-v1` | 소스 코드 (언어 무관) |
| `markdown-v1` | Markdown 텍스트 |
| `binary-v1` | 임의 바이너리 |

---

## 기여하기

커스텀 executor, trigger, skill은 독립적인 git 저장소로 공유할 수 있습니다. 스킬 패키징 형식은 [YAML 레퍼런스](yaml-reference.md)를 참조하세요.
