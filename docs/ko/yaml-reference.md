# Roster YAML Reference

모든 설정 파일은 `kind:` 필드로 타입을 선언합니다.
파일명 규칙: `{id}.{kind}.yaml`

---

## kind: org

루트 컨테이너. 프로젝트당 하나.

```yaml
# my-company.org.yaml
kind: org
id: my-company
name: My Company
subscribe: [project.requested]
emit: [project.done]
store:
  backend: sqlite        # sqlite | file | memory  (기본값: file)
  path: .roster/data.db
cron:
  - schedule: "5m"
    event: "heartbeat"
limits:
  max_iterations: 10
  cooldown: "5m"
budget:
  max_per_run: 10.0
  max_daily: 100.0
  max_monthly: 1000.0
```

| 필드 | 설명 |
|------|------|
| `subscribe` | 이 Org가 받을 이벤트 타입 목록 |
| `emit` | 이 Org가 완료 시 발행할 이벤트 타입 목록 |
| `store.backend` | 상태 저장 백엔드 |
| `store.path` | sqlite 경로 |
| `cron` | 주기적 이벤트 발행 스케줄 |
| `limits.max_iterations` | 서킷 브레이커 트립 전 이벤트 타입별 최대 발행 횟수 |
| `limits.cooldown` | 트립 후 억제 지속 시간 (예: `"5m"`) |
| `budget` | 조직 레벨 비용 한도 (USD) |

> Desk는 `groups:` 필드로 소속을 선언합니다. Group은 `parent:`로 중첩할 수 있습니다. Org가 자식을 열거하지 않습니다.

---

## kind: group

세션 공유 범위. Desk가 `groups` 필드로 소속을 선언합니다. Group은 `parent`로 중첩 가능.

```yaml
# dev-team.group.yaml
kind: group
id: dev-team
name: Dev Team
parent: engineering       # 선택: 상위 Group ID로 중첩
resources:
  - codebase              # 그룹 전체가 공유하는 리소스
```

| 필드 | 설명 |
|------|------|
| `parent` | 상위 Group ID (그룹 중첩용) |
| `resources` | 그룹 전체 공유 리소스 |

> Group은 이벤트를 subscribe/emit하지 않습니다 — Desk만 합니다. Group은 세션 공유 범위와 리소스 상속을 정의합니다.

---

## kind: desk

Agent가 앉는 실행 단위. `groups`로 소속 선언.

```yaml
# developer.desk.yaml
kind: desk
id: developer
groups:                    # 소속 그룹 (배열 — 여러 그룹 가능)
  - dev-team
agent: claude-cli         # Agent ID

role: "시니어 Go 개발자"
goal: "단일 책임 원칙으로 Roster 코드 작성"

skills:
  - go-developer          # skills/{name}.yaml 에서 해결
resources:
  - codebase

subscribe: [task.planned]
emit: [code.done]

executor:
  type: sdk               # api | exec | docker | remote | human | sdk
  env:
    ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
    CLAUDE_WORK_DIR: "${MY_PROJECT_DIR}"

session:
  max_entries: 20
  max_knowhow: 10

budget:
  max_per_run: 5.0
  max_daily: 50.0
```

| 필드 | 설명 |
|------|------|
| `groups` | 소속 Group ID 목록 (배열) |
| `agent` | Agent ID |
| `role` | 에이전트 페르소나 — 시스템 프롬프트 자동 조립 |
| `goal` | 에이전트 목표 — 시스템 프롬프트 자동 조립 |
| `skills` | 로드할 Skill 목록 (프롬프트에 포함) |
| `resources` | 에이전트가 접근할 Resource 목록 |
| `subscribe` | 받을 이벤트 타입 목록 |
| `emit` | 발행할 이벤트 타입 목록 (허용 목록) |
| `executor` | 실행 백엔드 설정 |
| `session.max_entries` | 세션 히스토리 최대 항목 수 |
| `session.max_knowhow` | 축적된 knowhow 최대 항목 수 (에이전트가 출력에 `## Knowhow` 섹션을 작성하면 허브가 추출하여 다음 실행 시 재주입) |
| `timeout` | 최대 실행 시간 (Go duration 문자열, 예: `"5m"`). 기본값: `30m`. Human 데스크는 기본 타임아웃 없음 |
| `budget.max_per_run` | 실행당 최대 USD |
| `budget.max_daily` | 24시간 롤링 윈도우 최대 USD |
| `budget.max_monthly` | 30일 롤링 윈도우 최대 USD |

### Executor 타입

| type | 설명 | 주요 env/params |
|------|------|-----------------|
| `api` | Anthropic / OpenAI / Gemini API 직접 호출 | `sdk`, `model` |
| `exec` | stdin/stdout JSON 프로토콜 커맨드 | `command` |
| `docker` | Docker 컨테이너 | `image` |
| `remote` | gRPC 원격 워커 | `address` |
| `human` | 웹 UI에서 사람이 직접 처리 | — |
| `sdk` | Python/Node SDK 에이전트 프로세스 (gRPC) | — |

### `sdk` executor — 빌트인 에이전트

`executor.type: sdk` 사용 시 `agent` 필드는 SDK 프로세스에 등록된 에이전트 ID를 참조합니다.

| agent ID | 설명 |
|----------|------|
| `claude-cli` | `claude` CLI 위임. `role`/`goal`/`skills`를 시스템 프롬프트로 조립 |

`claude-cli` 전용 env:

| 변수 | 설명 |
|------|------|
| `CLAUDE_WORK_DIR` | claude 실행 시 작업 디렉터리 |
| `CLAUDE_MODEL` | 모델 오버라이드 |
| `CLAUDE_SYSTEM_PROMPT` | 수동 시스템 프롬프트 (`role`/`goal`이 있으면 무시됨) |

### 사람도 Desk

```yaml
kind: desk
id: approval-gate
groups: [dev-team]
executor:
  type: human
subscribe: [review.done]
emit: [approved, rejected]
```

---

## kind: agent

로직 단위. 마켓플레이스에서 판매/구매하는 단위.

```yaml
# claude-cli.agent.yaml
kind: agent
id: claude-cli
name: Claude CLI
description: "Claude CLI 기반 에이전트"
sdk: "pip:roster-sdk"     # pip:{package} | local:{path} | git:{repo}
```

| 필드 | 설명 |
|------|------|
| `sdk` | SDK 패키지 위치 |
| `skills` | 에이전트 기본 Skill 목록 (Desk의 skills와 합산) |

> **subscribe/emit은 Desk가 선언합니다.** Agent는 순수 로직만 담습니다.

---

## kind: resource

외부 시스템 연결 정보. 로직 없음, 설정만.

```yaml
# codebase.resource.yaml
kind: resource
id: codebase
name: Codebase
type: local
config:
  path: ./roster

# figma.resource.yaml
kind: resource
id: figma
type: mcp
mcp: "npx @modelcontextprotocol/server-figma"

# db.resource.yaml
kind: resource
id: db
type: remote
connection: "${DATABASE_URL}"
```

| 필드 | 설명 |
|------|------|
| `type` | `local` \| `mcp` \| `remote` \| 임의 문자열 |
| `mcp` | MCP 서버 시작 커맨드 |
| `connection` | DB URL, API endpoint 등 |
| `config` | 에이전트에 그대로 전달되는 임의 key-value |
| `watch` | 파일 변경 이벤트 필터 glob 패턴 (예: `["**/*.go"]`). `local` 리소스의 경우 `config.path` 하위를 재귀적으로 감시. 생략 시 모든 변경이 이벤트 발생 |

> Resource는 설정만 합니다. 모든 상호작용 로직은 Agent가 담당합니다.

---

## kind: skill

에이전트에 로드되는 프롬프트 패키지.

```yaml
# go-developer.skill.yaml
kind: skill
name: go-developer
version: "1.0"
prompt: |
  ## 원칙
  - 단일 책임 원칙 엄수
  - 순환 의존 없음
  - 파일당 1000줄 이하
```

`.md` 파일도 사용 가능 — 파일 전체가 prompt로 사용됩니다.

**해결 순서:**
1. `skills/{name}.yaml`
2. `{name}.yaml`
3. `knowhow/{name}.yaml`
4. `{name}.md`
5. 원격: `github.com/org/repo/skill-name` 또는 `https://...`

---

## 환경변수 치환

YAML 값 어디서든 `${VAR_NAME}` 문법으로 환경변수를 참조합니다.

```yaml
env:
  ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
```

---

## 프로젝트 구조

```
my-company/
├── my-company.org.yaml
├── dev-team.group.yaml
├── developer.desk.yaml
├── reviewer.desk.yaml
├── human-gate.desk.yaml
├── claude-cli.agent.yaml
├── codebase.resource.yaml
└── skills/
    └── go-developer.skill.yaml
```

모든 파일이 한 폴더에 있어도 됩니다. Hub이 자동으로 탐색합니다.

---

## 이벤트 흐름

```
이벤트 발생
→ subscribe 중인 Desk 깨어남
→ Desk: Agent 실행 → emit으로 완료 신호
→ 출력은 세션 히스토리에 기록; 데이터 공유는 리소스를 통해
```
