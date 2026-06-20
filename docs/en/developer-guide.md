# Roster Developer Guide

Regular users can work with Roster using only YAML.
Developers can build custom executors, triggers, and adapters to run on top of Roster.

---

## Three Ways to Extend

| method | target | language | description |
|--------|--------|----------|-------------|
| **Exec Protocol** | Executor | any | stdin JSON → stdout JSON |
| **gRPC Worker** | Executor | any | implement the Roster worker service |
| **Go SDK** | Executor/Trigger/Adapter | Go | implement `sdk.Executor`, `sdk.Trigger` interfaces |

---

## 1. Exec Protocol — The Easiest Way

You can build a Roster executor in any language.

### Desk Configuration

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

### stdin Format (hub → executor)

```json
{
  "agent_id": "researcher",
  "desk_id": "researcher-desk",
  "prompt": "<merged skill prompts + input context>",
  "session": [
    {"role": "user", "content": "previous task context"},
    {"role": "assistant", "content": "previous response"}
  ],
  "group_history": [
    {"desk_id": "designer-desk", "role": "assistant", "content": "Design is complete"}
  ],
  "input": "<output text from the previous step, empty string if none>"
}
```

### stdout Format (executor → hub)

```json
{
  "content": "execution result text",
  "metrics": {"tokens_used": 150}
}
```

- `content`: the output text (saved to session)
- `metrics`: optional key-value metrics

**Raw stdout fallback**: if the output is not valid JSON, the entire stdout is treated as the content.

### Example: Python executor

```python
#!/usr/bin/env python3
import sys, json

data = json.load(sys.stdin)

prompt = data["prompt"]
session = data.get("session", [])
group_history = data.get("group_history", [])

# put any AI/logic you want here
result = call_my_llm(prompt, session)

print(json.dumps({"schema": "text-v1", "payload": result}))
```

### Example: Node.js executor

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

## 2. gRPC Worker — Remote Execution

Use this when you need to run agents on a separate server (a GPU server, a server inside a private network, etc.).

### Starting the Worker Server

```bash
roster worker :50051
```

### Desk Configuration

```yaml
kind: desk
id: gpu-agent-desk
agent: my-agent
executor:
  type: remote
  params:
    address: "gpu-server.internal:50051"
```

### Implementing a Custom Worker Server

You can implement a worker in any language using `proto/worker.proto`.

```protobuf
service Worker {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

**Python example (grpc):**

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

## 3. Go SDK — The Most Powerful Way

Writing in Go lets you import Roster as a library and build a fully customized hub.

### Custom Executor

```go
import "github.com/roster-io/roster/pkg/sdk"

type MyExecutor struct{}

func (e *MyExecutor) Run(ctx context.Context, task sdk.Task) (*types.Output, error) {
    // task.Prompt        — what to execute
    // task.Session       — the desk's previous conversation history
    // task.GroupHistory  — messages in the team's shared space
    // task.Resources     — available resources with config
    // task.Skills        — resolved skill prompts
    // task.Options       — executor config from the desk YAML
    // task.Env           — environment variables

    result := callMyService(task.Prompt)
    return &types.Output{Content: result}, nil
}
```

Registering (in main.go):

```go
registry := runner.NewRegistry()
registry.Register("my-custom", &MyExecutor{})
```

Using in desk YAML:

```yaml
executor:
  type: my-custom
  params:
    endpoint: "https://my-service.com/api"
```

---

## SDK Interface Reference

### `sdk.Executor`

```go
type Executor interface {
    Run(ctx context.Context, task Task) (*types.Output, error)
}
```

### `sdk.Task`

```go
type Task struct {
    RunID     string
    AgentID   string
    DeskID    string
    GroupID   string            // empty if desk is not inside a group
    EventType string            // the event type that triggered this desk
    Prompt    string            // skill prompts merged + input context
    Options   map[string]string // executor configuration (command, image, sdk, etc.)
    Env       map[string]string // environment variables
    WorkDir   string            // working directory for exec runner

    Notes       map[string][]byte  // current note store snapshot for this scope
    Session     []SessionEntry     // desk's persistent session history
    GroupHistory []GroupMessage    // team's shared communication history
    Resources   []TaskResource     // resources available to this desk
    Skills      map[string]string  // skill name → resolved prompt content
}
```

### `sdk.TaskResource`

```go
type TaskResource struct {
    ID     string            // resource ID
    Type   string            // resource type (mcp, local, remote, etc.)
    Config map[string]string // resource configuration (path, connection, etc.)
}
```

---

## Contributing

Custom executors and skills can be shared as standalone git repositories. See the [YAML reference](yaml-reference.md) for the skill packaging format.
