# Roster YAML Reference

모든 설정 파일은 `kind:` 필드로 타입을 선언합니다.
파일은 어디에 있어도 됩니다 — 허브가 자동으로 찾습니다.

---

## kind: agent

에이전트의 정체성. 어떤 스킬을 갖고 있는지 정의합니다.

```yaml
kind: agent
id: researcher           # 고유 ID (필수)
name: Researcher
skills:
  - web-search           # 로컬 skills/ 폴더에서 찾음
  - summarize            # skills/summarize.yaml
  - github.com/org/roster-skills/translate-v1   # 커뮤니티 스킬
  - https://skills.roster.io/code-review-v1     # URL 스킬
```

인라인 agent 선언: desk 파일 안에서 `agent:` 필드를 문자열(ID 참조) 대신 매핑으로 쓰면 에이전트를 데스크와 함께 정의할 수 있습니다.

---

## kind: desk

에이전트의 실행 환경. 이벤트를 받아 작업을 수행하고 이벤트를 emit합니다.

```yaml
kind: desk
id: researcher-desk      # 고유 ID (필수)
agent: researcher        # 에이전트 ID 또는 상대 경로 (./path/agent.yaml)

executor:
  type: api              # api | exec | docker | remote | human
  sdk: anthropic         # api 전용: anthropic | openai | gemini
  params:
    model: claude-sonnet-4-6
    command: "python script.py"   # exec 전용
    image: "my-org/agent:latest"  # docker 전용
    address: "host:50051"         # remote 전용
  env:
    api_key: "${ANTHROPIC_API_KEY}"

concurrency:
  mode: queue            # queue | spawn | reject
  max: 3                 # spawn 전용: 최대 동시 실행 수

subscribe:               # 이 데스크가 수신할 이벤트 타입 목록
  - plan.approved
  - code.ready

emit:                    # 작업 완료 시 emit할 이벤트 타입
  - research.done

# TIP: `emit`에 나열된 이벤트는 데스크의 executor가 완료될 때
# (스크립트 종료, API 응답 반환, 프로세스 완료) 자동으로 발행됩니다.
# LLM이 생성 중에 emit을 결정하게 하기보다는, 스크립트나 프로세스가
# 끝났을 때 명시적으로 반환하는 방식을 권장합니다.
# 이벤트 흐름이 결정적(deterministic)이고 재현 가능해집니다.

cron: "0 */6 * * *"     # 선택: cron 스케줄로 자동 실행 (6시간마다)

resources:               # 이 데스크만 접근 가능한 리소스
  - github-api

tags:                    # 역할 기반 퍼미션 매칭용 태그
  - backend
  - senior

policy: careful          # 선택: 정책 참조 (retry/timeout/cost)
```

### Executor 타입

| type | 설명 | 필수 params |
|------|------|-------------|
| `api` | 내장 AI SDK | `sdk`, `model` |
| `exec` | 임의 커맨드 (stdin/stdout JSON) | `command` |
| `docker` | Docker 컨테이너 | `image` |
| `remote` | gRPC 원격 워커 | `address` |
| `human` | 사람이 웹 UI에서 직접 수행 | 없음 |

### Concurrency 모드

| mode | 동작 |
|------|------|
| `queue` | 요청을 순서대로 처리 (기본값) |
| `spawn` | 동시에 `max`개까지 실행 |
| `reject` | 이미 실행 중이면 거절 |

---

## kind: group

팀. 데스크들의 묶음. 이벤트를 받으면 활성화되고, 멤버 데스크들이 공유 컨텍스트(그룹 세션)로 협력합니다.

```yaml
kind: group
id: dev-team             # 고유 ID (필수)
name: Dev Team

lead:
  desk: architect        # 그룹을 조율하는 리드 데스크
  position: both         # both (기본) | first | last

desks:
  - frontend-desk
  - backend-desk

groups:                  # 중첩 그룹 (선택)
  - design-team

resources:               # 그룹 전체가 공유하는 리소스
  - codebase
  - backlog

subscribe:               # 이 그룹이 수신할 이벤트 타입
  - plan.approved

emit:                    # 작업 완료 시 emit할 이벤트 타입
  - code.ready           # (위 Desk emit TIP 참고 — 동일 원칙 적용)

cron: "0 */3 * * *"     # 선택: cron 스케줄로 자동 실행

policy: standard         # 선택: 정책 참조

dispatch: sequential     # sequential (기본) | parallel | conversation
```

### Lead Position

| position | 실행 순서 |
|----------|----------|
| `both` (기본) | lead → members → lead (plan/assign → work → synthesize) |
| `first` | lead → members (lead가 업무를 분해해서 members에게 전달) |
| `last` | members → lead (members 결과를 lead가 취합) |

### Dispatch 모드

| dispatch | 동작 |
|----------|------|
| `sequential` (기본) | 멤버를 순서대로 실행. 앞 결과가 다음 입력으로 |
| `parallel` | 모든 멤버에게 같은 입력 → 동시 실행 → 결과 합산 (미구현, 예정) |
| `conversation` | 멀티 라운드 순차 실행. 멤버들이 서로 대화 |

---

## kind: organization

최상위 시스템 정의. 그룹 구성과 이벤트 라우팅을 선언합니다.
프로젝트당 하나만 선언합니다.

```yaml
kind: organization
id: engineering          # 고유 ID (필수)
name: Engineering Org

groups:
  - strategy-team
  - dev-team
  - ops-team

resources:               # 조직 전체가 공유하는 리소스
  - codebase
  - slack

routing:
  - on: plan.approved    # 이 이벤트를 수신하면
    to: dev-team         # 이 그룹(또는 데스크)으로 전달
  - on: code.ready
    to: ops-team
  - on: hub.started      # 허브 시작 시 자동 발행되는 이벤트
    to: strategy-team
```

라우팅 규칙은 조직 레벨 외에 각 그룹/데스크의 `subscribe` 필드로도 선언할 수 있습니다.
두 방식은 동일하게 동작합니다.

---

## kind: resource

외부 시스템 연결. 파일, git, API 등 어떤 외부 상태든 리소스로 선언할 수 있습니다.

```yaml
kind: resource
id: codebase             # 고유 ID (필수)
name: Codebase
type: file               # 리소스 타입 (file, github, 사용자 정의 등)

config:
  root: .                # 리소스 루트 경로
  path: roster/          # 작업 대상 경로

watch:                   # 변경 감지 시 emit할 이벤트 타입
  - file.changed

interval: "5m"           # watch 폴링 간격 (기본 60s)

actions:
  read:
    exec: scripts/read-files.sh
    description: Read file contents
  commit:
    exec: scripts/git-commit.sh
    description: Commit changes to git

permissions:
  - allow: ["*"]         # 모든 액션 허용
    groups: [dev-team, ops-team]
  - allow: [read]
    tags: [viewer]
```

데스크가 리소스 액션을 호출하려면 exec runner에서 stderr로 `ACTION:` 프로토콜을 사용합니다.
자세한 내용은 developer guide의 exec protocol 섹션을 참조하세요.

### 퍼미션 규칙

퍼미션이 없으면 모든 데스크/그룹이 모든 액션에 접근할 수 있습니다.
퍼미션이 있으면 매칭되는 규칙이 있는 데스크/그룹만 접근 가능합니다.

| 필드 | 설명 |
|------|------|
| `allow` | 허용할 액션 목록. `"*"` 은 전체 허용 |
| `desks` | 데스크 ID 목록 |
| `groups` | 그룹 ID 목록 |
| `tags` | 데스크 태그 목록 |

---

## kind: policy

데스크/그룹에 적용할 운영 규칙. retry, timeout, 비용 한도를 설정합니다.

```yaml
kind: policy
id: careful              # 고유 ID (필수)
name: Careful Policy

retry: 3                 # 실패 시 최대 재시도 횟수
timeout: 5m              # 최대 실행 시간 (e.g. 30s, 5m, 1h)
cost_limit: "$0.10"      # 호출당 최대 비용 한도
```

데스크/그룹에 적용:
```yaml
kind: desk
id: expensive-desk
policy: careful
```

---

## kind: skill

에이전트가 사용하는 프롬프트 패키지.

```yaml
kind: skill
id: web-search-v1
name: Web Search
version: "1"
description: Search the web for relevant information

prompt: |
  You are a research assistant. When given a topic or question,
  search for the most relevant and recent information.

  Format your response as:
  1. Key findings
  2. Sources
  3. Summary
```

Markdown(`.md`) 또는 텍스트(`.txt`) 파일도 스킬로 사용할 수 있습니다 — `prompt:` 필드 없이 파일 내용 전체가 프롬프트로 사용됩니다.

---

## kind: pipeline

순차 실행 파이프라인. 노드를 명시적 순서로 실행합니다.

> **참고**: pipeline은 레거시 모델입니다. 새 프로젝트에는 organization + subscribe/emit 이벤트 기반 모델을 권장합니다.

```yaml
kind: pipeline
id: feature-launch       # 고유 ID (필수)

trigger:                 # 자동 트리거 (없으면 수동 실행)
  type: exec             # exec | polling | webhook | manual
  command: "./check.py"
  interval: "5m"

steps:
  - desk: me
    label: requirements

  - agent: researcher
    trigger:
      from: [requirements]

  - group: dev-team
    trigger:
      from: [researcher]
    gate: approve        # 웹 UI에서 승인/거절

  - desk: deployer-desk
    trigger:
      from: [dev-team]
```

---

## 환경 변수 치환

모든 YAML 값에서 `${VAR_NAME}` 형식으로 환경 변수를 참조할 수 있습니다.

```yaml
env:
  api_key: "${ANTHROPIC_API_KEY}"
  repo: "${GITHUB_REPO}"
```

---

## 프로젝트 구조 예시

파일 위치는 자유롭습니다. `kind:` 필드가 있으면 허브가 자동으로 찾습니다.

```
my-org/
├── organization.yaml     # kind: organization
├── agents/
│   └── architect.yaml    # kind: agent (또는 desks/ 안에 인라인)
├── desks/
│   ├── architect.yaml    # kind: desk
│   └── implementer.yaml
├── groups/
│   ├── strategy-team.yaml  # kind: group
│   └── dev-team.yaml
├── resources/
│   ├── codebase.yaml     # kind: resource
│   └── slack.yaml
├── policies/
│   └── standard.yaml     # kind: policy
└── skills/
    └── code-review.md    # 스킬 프롬프트
```
