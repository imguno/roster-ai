# Roster YAML 스키마 레퍼런스

모든 Roster 설정 파일은 `kind` 필드를 선언합니다. Hub는 이 필드만으로 임의의 디렉터리 구조에서 파일을 찾아 로드합니다.

## 유효한 Kind 목록

| Kind | 설명 |
|---|---|
| `organization` | 최상위 시스템: group, 라우팅, 스토어, 기본값 |
| `group` | 이벤트 스트림을 공유하는 desk 팀 |
| `desk` | 독립적인 actor: 이벤트를 받고, 작업을 실행하고, 이벤트를 emit |
| `agent` | 정체성: 역할 설명, skill, knowhow |
| `resource` | 외부 시스템과의 연결 (GitHub, Slack, 커스텀) |
| `policy` | 운영 규칙: retry, timeout, 예산, escalation |
| `skill` | 프롬프트 조각 (`name`, `version`, `prompt`를 포함하는 YAML) |

## 파일 이름과 ID 도출

`id:` 필드가 생략된 경우(일반적인 경우), ID는 파일 이름에서 자동으로 도출됩니다:

| 파일 경로 | 도출된 ID | 규칙 |
|---|---|---|
| `desks/reviewer.yaml` | `reviewer` | 파일 이름의 stem 사용 |
| `desks/reviewer/desk.yaml` | `reviewer` | stem이 kind와 같으면 부모 디렉터리 이름 사용 |
| `groups/dev-team.yaml` | `dev-team` | 동일한 stem 규칙 |
| `agents/senior-dev/agent.yaml` | `senior-dev` | 동일한 부모 디렉터리 규칙 |

규칙: 확장자를 제거해 stem을 구합니다. stem이 kind 이름과 같으면 (예: kind가 `desk`인 파일의 이름이 `desk.yaml`인 경우), 대신 부모 디렉터리 이름을 사용합니다.

## 암묵적 Agent 바인딩

Desk 이름이 agent ID와 일치하면 자동으로 바인딩됩니다. 명시적인 `agent:` 필드가 필요 없습니다.

예시: ID가 `reviewer`인 desk와 ID가 `reviewer`인 agent는 자동으로 연결됩니다. Desk의 `agent` 필드는 이름이 다를 때나 인라인으로 agent를 정의할 때만 필요합니다.

## 기본값 상속

Desk 설정은 다음 순서로 기본값을 상속받습니다 (나중에 오는 것이 앞의 것을 덮어씁니다):

```
Organization 기본값  ->  Group 기본값  ->  Desk 레벨 설정
```

`organization.defaults`와 `group.defaults` 모두 동일한 `DeskDefaults` 구조를 사용합니다:

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Executor | `executor` | ExecutorConfig | 모든 desk의 기본 executor |
| Policy | `policy` | string | 기본 policy 참조 |
| Tags | `tags` | string[] | Desk에 적용되는 기본 태그 |

---

## Organization

프로젝트당 하나. Group, 라우팅, 스토리지, 기본값을 정의합니다.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Kind | `kind` | string | 예 | `"organization"` |
| ID | `id` | string | | 파일 이름에서 자동 도출 |
| Name | `name` | string | | 사람이 읽기 쉬운 이름 |
| Description | `description` | string | | |
| Groups | `groups` | string[] | | 이 org에 속하는 group ID 목록 |
| Resources | `resources` | string[] | | org 레벨 resource ID 목록 |
| Routing | `routing` | RoutingRule[] | | 이벤트 라우팅 테이블 |
| Store | `store` | StoreConfig | | 스토리지 백엔드 설정 |
| Defaults | `defaults` | DeskDefaults | | 모든 desk의 기본값 |

### RoutingRule

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| On | `on` | string | 예 | 매칭할 이벤트 타입 |
| To | `to` | string | 예 | 대상 group 또는 desk ID |
| When | `when` | string | | 라우팅 조건식 |

### StoreConfig

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Backend | `backend` | string | `"file"` (기본값), `"sqlite"`, `"memory"` |
| Path | `path` | string | 데이터 디렉터리 또는 DB 경로. 기본값: `.roster/data` |

### 예시

```yaml
kind: organization
name: engineering
description: Engineering organization with three teams  # 세 팀으로 구성된 엔지니어링 조직

groups:
  - strategy-team
  - dev-team
  - ops-team

resources:
  - github-app
  - slack-org

defaults:
  executor:
    type: exec
    params:
      command: scripts/claude-code.sh
    env:
      CLAUDE_MODEL: claude-sonnet-4-6
  policy: standard
  tags: [team-member]

store:
  backend: sqlite
  path: .roster/data/roster.db

routing:
  - on: task.created
    to: strategy-team
  - on: plan.ready
    to: dev-team
  - on: code.ready
    to: ops-team
    when: "event.labels contains 'approved'"
```

---

## Group

이벤트 스트림을 공유하는 desk 팀. 모든 desk가 다른 desk의 결과물을 볼 수 있습니다.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Kind | `kind` | string | 예 | `"group"` |
| ID | `id` | string | | 파일 이름에서 자동 도출 |
| Name | `name` | string | | 사람이 읽기 쉬운 이름 |
| Description | `description` | string | | |
| Lead | `lead` | GroupLead | | 조율자 desk 설정 |
| Desks | `desks` | string[] | | 멤버 desk ID 목록 |
| Groups | `groups` | string[] | | 중첩 서브 group ID 목록 |
| Resources | `resources` | string[] | | group 레벨 resource ID 목록 |
| Subscribe | `subscribe` | string[] | | 이 group이 구독하는 이벤트 타입 목록 |
| Emit | `emit` | string[] | | 이 group이 생성하는 이벤트 타입 목록 |
| Cron | `cron` | string | | Cron 스케줄 (예: `"0 */3 * * *"`) |
| Policy | `policy` | string | | Policy 참조 |
| Dispatch | `dispatch` | string | | `"sequential"` (기본값), `"parallel"`, `"conversation"` |
| Triggers | `triggers` | TriggerConfig[] | | 자동화된 이벤트 소스 |
| Defaults | `defaults` | DeskDefaults | | 이 group의 desk에 대한 org 기본값 덮어쓰기 |

### GroupLead

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Desk | `desk` | string | 예 | Lead desk ID |
| Position | `position` | string | | `"both"` (기본값), `"first"`, `"last"` |

**Lead 위치:**

| Position | 패턴 |
|---|---|
| `both` (기본값) | lead 계획 -> 멤버 작업 -> lead 종합 |
| `first` | lead 분해 -> 멤버 실행 |
| `last` | 멤버 작업 -> lead 종합 |
| _(생략)_ | Lead 없음; 멤버가 직접 작업 |

**Dispatch 모드:**

| 모드 | 설명 |
|---|---|
| `sequential` | 멤버가 하나씩 순서대로 실행되며, 각 멤버는 이전 결과물을 볼 수 있음 (기본값) |
| `parallel` | 모든 멤버가 동일한 입력으로 동시에 실행되고, 결과가 합쳐짐 |
| `conversation` | 두 라운드의 순차 실행; 멤버들이 서로에게 응답 |

### 예시

```yaml
kind: group
name: dev-team
description: Development team with lead, implementers, and reviewer  # lead, 구현자, 리뷰어로 구성된 개발 팀

lead:
  desk: architect
  position: both

desks:
  - implementer-a
  - implementer-b
  - reviewer

groups:
  - testing-subteam

resources:
  - codebase

subscribe:
  - plan.ready
emit:
  - code.ready

cron: "0 9 * * 1"

policy: team-policy
dispatch: sequential

triggers:
  - type: poll
    url: https://ci.example.com/status
    interval: 5m
    event: ci.updated

defaults:
  executor:
    type: exec
    params:
      command: scripts/claude-code.sh
  policy: careful
  tags: [dev]
```

---

## Desk

독립적인 actor. 이벤트를 받고, 한 가지 작업을 수행하고, 이벤트를 emit합니다.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Kind | `kind` | string | 예 | `"desk"` |
| ID | `id` | string | | 파일 이름에서 자동 도출 |
| Name | `name` | string | | 사람이 읽기 쉬운 이름 |
| Description | `description` | string | | |
| Executor | `executor` | ExecutorConfig | 예 | 이 desk의 실행 방식 |
| Concurrency | `concurrency` | ConcurrencyConfig | | 병렬 실행 제어 |
| Subscribe | `subscribe` | string[] | | 이 desk가 구독하는 이벤트 타입 목록 |
| Emit | `emit` | string[] | | 이 desk가 생성하는 이벤트 타입 목록 |
| Cron | `cron` | string | | Cron 스케줄 (예: `"*/30 * * * *"`) |
| Resources | `resources` | string[] | | 이 desk에 바인딩된 전용 resource ID 목록 |
| Tags | `tags` | string[] | | 역할 기반 권한 태그 (예: `["backend", "senior"]`) |
| Policy | `policy` | string | | Policy 참조 |
| Triggers | `triggers` | TriggerConfig[] | | 자동화된 이벤트 소스 |
| Session | `session` | SessionConfig | | 세션 히스토리 동작 |

참고: `agent` 필드는 YAML 필드가 아닙니다 (yaml 태그는 `"-"`). Agent 바인딩은 이름 매칭 또는 인라인 정의로 이루어집니다 (위의 암묵적 Agent 바인딩 참고).

### ExecutorConfig

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Type | `type` | string | 예 | `"api"`, `"exec"`, `"docker"`, `"remote"`, `"human"` |
| SDK | `sdk` | string | | type=api일 때 AI SDK: `"anthropic"`, `"openai"`, `"gemini"` |
| Address | `address` | string | | remote executor의 엔드포인트 |
| Params | `params` | map[string]string | | Executor별 파라미터 |
| Env | `env` | map[string]string | | 환경 변수 |

**Executor 타입:**

| 타입 | 설명 | 주요 params/필드 |
|---|---|---|
| `api` | 내장 AI SDK | `sdk`, `params`에 model/설정 |
| `exec` | stdin/stdout을 통한 임의 명령 | `params`의 `command`, `env` |
| `docker` | Docker 컨테이너 | `params`의 `image` |
| `remote` | gRPC를 통한 원격 워커 | `address` |
| `human` | 웹 UI를 통한 사람 참여자 | |

### ConcurrencyConfig

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Mode | `mode` | string | `"queue"` (기본값), `"spawn"`, `"reject"` |
| Max | `max` | int | 최대 병렬 워커 수 (`spawn` 모드에서 사용) |

**Concurrency 모드:**

| 모드 | 설명 |
|---|---|
| `queue` | 요청을 큐에 넣음 (기본값) |
| `spawn` | `max`까지 병렬 워커를 생성 |
| `reject` | 바쁠 때 요청을 거절 |

### TriggerConfig

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Type | `type` | string | `"exec"`, `"poll"` |
| Command | `command` | string | 실행할 명령 (exec 타입). exit code 0일 때 이벤트 발화 |
| URL | `url` | string | GET할 URL (poll 타입). 상태 200일 때 이벤트 발화 |
| Interval | `interval` | string | 확인 간격 (기본값: `"30s"`) |
| Event | `event` | string | 트리거 시 emit할 이벤트 타입 |

### SessionConfig

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| MaxEntries | `max_entries` | *int | 컨텍스트로 로드할 최대 세션 항목 수. 기본값: 40. 0으로 설정하면 비활성화 |

### 예시

```yaml
kind: desk
name: reviewer
description: Reviews code changes for quality and security  # 품질과 보안을 위한 코드 변경 리뷰

executor:
  type: exec
  params:
    command: scripts/claude-code.sh
  env:
    CLAUDE_MODEL: claude-sonnet-4-6

concurrency:
  mode: spawn
  max: 3

subscribe:
  - code.submitted
emit:
  - review.done

cron: "0 9 * * *"

resources:
  - codebase
  - github-app

tags:
  - backend
  - senior

policy: careful

triggers:
  - type: exec
    command: scripts/check-new-prs.sh
    interval: 2m
    event: pr.needs_review
  - type: poll
    url: https://ci.example.com/pending-reviews
    interval: 5m
    event: review.pending

session:
  max_entries: 20
```

---

## Agent

정체성 정의: 이 사람이 누구인지, 무엇을 아는지.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Kind | `kind` | string | 예 | `"agent"` |
| ID | `id` | string | | 파일 이름에서 자동 도출 |
| Name | `name` | string | | 사람이 읽기 쉬운 이름 |
| Description | `description` | string | | 역할 설명 |
| Skills | `skills` | string[] | | Skill 참조 목록 (아래 Skill 참조 형식 참고) |
| Knowhow | `knowhow` | string[] | | Knowhow 문서 참조 목록 |

### Skill 참조 형식

Skill은 다음 형식으로 참조할 수 있습니다:

| 형식 | 예시 | 해석 방식 |
|---|---|---|
| 이름만 | `code-review` | 로컬 레지스트리에서 검색 |
| Git 경로 | `github.com/org/repo/skill-name` | git에서 가져옴 |
| HTTPS URL | `https://example.com/skill.yaml` | HTTP로 가져옴 |

### 예시

```yaml
kind: agent
name: senior-reviewer
description: Senior engineer focused on code quality and security  # 코드 품질과 보안에 집중하는 시니어 엔지니어

skills:
  - code-review
  - security-checklist
  - github.com/roster-community/skills/go-best-practices

knowhow:
  - common-bugs
  - past-incidents
```

---

## Skill

Skill은 프롬프트 내용을 담은 YAML 파일입니다.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Name | `name` | string | 예 | Skill 이름 |
| Version | `version` | string | 예 | 버전 문자열 |
| Prompt | `prompt` | string | 예 | 실행자에게 전달되는 지시 사항 |

Knowhow 파일은 `knowhow/` 디렉터리에 위치한 일반 마크다운(`.md`) 파일입니다. 파일 이름이 ID가 됩니다.

### 예시

```yaml
name: code-review
version: "1.0"
prompt: |
  You are a code reviewer. Focus on:  # 당신은 코드 리뷰어입니다. 다음에 집중하세요:
  - Correctness and edge cases         # 정확성과 엣지 케이스
  - Security vulnerabilities           # 보안 취약점
  - Performance implications           # 성능 영향
  - Code readability and maintainability  # 코드 가독성과 유지보수성
```

---

## Resource

외부 시스템과의 연결로, watch 이벤트와 action을 제공합니다.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Kind | `kind` | string | 예 | `"resource"` |
| ID | `id` | string | | 파일 이름에서 자동 도출 |
| Name | `name` | string | | 사람이 읽기 쉬운 이름 |
| Description | `description` | string | | |
| Type | `type` | string | | Resource 타입 (예: `"github"`, `"slack"`, `"custom"`) |
| Config | `config` | map[string]string | | 타입별 설정 |
| Watch | `watch` | string[] | | 감시할 이벤트 타입 목록 |
| Actions | `actions` | map[string]*ResourceAction | | Desk가 호출할 수 있는 named 작업 |
| Permissions | `permissions` | PermissionRule[] | | 접근 제어 규칙. 비어 있으면 모든 action이 공개 |
| Interval | `interval` | string | | watch 폴링 간격 (예: `"5m"`) |

### ResourceAction

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Exec | `exec` | string | 실행할 스크립트 |
| Skill | `skill` | string | Skill 기반 action (LLM 추론) |
| Description | `description` | string | 사람이 읽기 쉬운 설명 |
| Params | `params` | map[string]string | 정적 파라미터 |

### PermissionRule

매칭된 desk, group, 태그에 action 집합을 허용합니다. permission이 정의되지 않으면 모든 action이 누구에게나 공개됩니다.

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Allow | `allow` | string[] | 허용할 action 이름 목록. `"*"`는 모든 action |
| Desks | `desks` | string[] | 매칭할 desk ID 목록 |
| Groups | `groups` | string[] | 매칭할 group ID 목록 |
| Tags | `tags` | string[] | 매칭할 태그 값 목록 |

### 예시

```yaml
kind: resource
name: codebase
description: Main application repository  # 메인 애플리케이션 저장소
type: github

config:
  repo: acme/app
  token_env: GITHUB_TOKEN

watch:
  - pull_request
  - issue

interval: 5m

actions:
  commit:
    exec: scripts/commit.sh
    description: Commit changes to the repo  # 저장소에 변경사항 커밋
  search:
    skill: search-codebase
    description: Search the codebase using LLM reasoning  # LLM 추론으로 코드베이스 검색
  deploy:
    exec: scripts/deploy.sh
    description: Trigger a deployment  # 배포 트리거
    params:
      environment: staging

permissions:
  - allow: [commit, search]
    desks: [implementer-a, implementer-b]
  - allow: [commit, search, deploy]
    groups: [dev-team]
  - allow: [search]
    tags: [viewer]
  - allow: ["*"]
    desks: [admin]
```

---

## 내장 Roster Resource

모든 desk는 desk 간 통신을 위한 가상 `roster` resource에 자동으로 접근할 수 있습니다. 별도 설정이 필요 없습니다.

| Action | 파라미터 | 설명 |
|---|---|---|
| `call` | `desk` (필수), `prompt` (필수) | 대상 desk를 동기적으로 실행하고 결과를 반환. 자기 자신 호출은 거절됨 |
| `artifact` | `desk` (필수) | 해당 desk에 저장된 최신 artifact를 반환 |
| `session` | `desk` (필수), `limit` (선택) | 해당 desk의 최근 세션 항목을 반환 |

exec 프로토콜에서의 사용 (stderr를 통해):

```
ACTION:{"resource":"roster","action":"call","params":{"desk":"reviewer","prompt":"review this code"}}
ACTION:{"resource":"roster","action":"artifact","params":{"desk":"implementer"}}
ACTION:{"resource":"roster","action":"session","params":{"desk":"architect","limit":"5"}}
```

---

## Policy

retry, timeout, 비용, escalation에 대한 운영 규칙입니다.

| 필드 | YAML | 타입 | 필수 | 설명 |
|---|---|---|---|---|
| Kind | `kind` | string | 예 | `"policy"` |
| ID | `id` | string | | 파일 이름에서 자동 도출 |
| Name | `name` | string | | 사람이 읽기 쉬운 이름 |
| Description | `description` | string | | |
| Retry | `retry` | int | | 실패 시 최대 재시도 횟수 (기본값: 0) |
| Timeout | `timeout` | duration | | 최대 실행 시간 (예: `"5m"`, `"1h"`) |
| CostLimit | `cost_limit` | string | | 호출당 최대 비용 (예: `"$0.10"`) |
| EscalateTo | `escalate_to` | string | | 실패 시 escalate할 desk ID |
| OnTimeout | `on_timeout` | string | | `"fail"` (기본값), `"retry"`, `"escalate"` |
| OnError | `on_error` | string | | `"fail"` (기본값), `"retry"`, `"escalate"` |
| Budget | `budget` | BudgetConfig | | 다중 단위 비용 한도 |
| EscalationChain | `escalation_chain` | string[] | | 다단계 escalation: L1 -> L2 -> L3 desk ID |
| RequireSchema | `require_schema` | string | | artifact 스키마 강제 (예: `"json-v1"`, `"code-v1"`) |

### BudgetConfig

| 필드 | YAML | 타입 | 설명 |
|---|---|---|---|
| Total | `total` | string | 누적 비용 한도 (전체 기간). 형식: `"$500.00"` |
| PerRun | `per_run` | string | 단일 실행당 최대 비용. 형식: `"$5.00"` |
| Daily | `daily` | string | 24시간 롤링 윈도우당 최대 비용. 형식: `"$50.00"` |
| WarnAt | `warn_at` | float | 경고를 emit할 비율 (0.0-1.0). 예: `0.8` = 80%에서 경고 |

### 예시

```yaml
kind: policy
name: careful
description: Conservative policy with budget controls and escalation  # 예산 제어와 escalation을 갖춘 보수적인 policy

retry: 3
timeout: 5m
cost_limit: "$0.10"
on_timeout: escalate
on_error: retry
escalate_to: on-call

budget:
  total: "$500.00"
  per_run: "$5.00"
  daily: "$50.00"
  warn_at: 0.8

escalation_chain:
  - on-call
  - team-lead
  - engineering-manager

require_schema: json-v1
```

---

## 디렉터리 구조

파일은 자유롭게 구성할 수 있습니다. Hub는 디렉터리 구조가 아닌 `kind` 필드로 파일을 찾습니다. 일반적인 구조는 다음과 같습니다:

```
org/
  organization.yaml
  groups/
    strategy-team.yaml
    dev-team.yaml
  desks/
    implementer.yaml
    reviewer.yaml
    architect.yaml
  agents/
    senior-reviewer.yaml
  skills/
    code-review.yaml
  knowhow/
    common-bugs.md
  resources/
    codebase.yaml
    slack-dev.yaml
  policies/
    careful.yaml
  scripts/
    commit.sh
    claude-code.sh
```

## 세션 동작

세션은 자동으로 관리됩니다. YAML 설정 파일로 선언하지 않습니다.

| 범위 | 수명 | 내용 |
|---|---|---|
| Desk 세션 | 실행 간 지속 | Desk 자신의 히스토리와 학습된 컨텍스트 |
| Group 세션 | 매 실행마다 초기화 | 한 번의 group 실행 내 모든 desk의 결과물 |

Desk별로 `session.max_entries` 필드로 세션 로딩을 제어할 수 있습니다 (기본값: 40, 0으로 설정하면 비활성화).
