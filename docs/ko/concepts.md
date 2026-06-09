# Roster — 핵심 개념

Roster는 **Organization as Code**입니다.

Infrastructure as Code가 서버를 프로비저닝하듯, Roster는 조직을 프로비저닝합니다 — desk, group, resource, 라우팅을 YAML로 선언하고, git으로 버전 관리하며, 어디서든 재현할 수 있습니다.

AI를 실행하는 스크립트를 작성하는 게 아닙니다. 실제로 일하는 팀을 세우는 것입니다.

Roster의 모든 것은 아홉 가지 작은 개념으로 구성되어 있습니다. 이것들은 서로 조합됩니다.

---

## Event

**Event**는 범용 메시지입니다. 시스템을 흐르는 모든 것이 이벤트입니다 — 새 task가 들어오거나, desk가 작업을 마치거나, resource가 변경되거나, 사람이 응답하거나.

Desk는 이벤트를 구독합니다. Desk는 이벤트를 emit합니다. 이것이 전체 커뮤니케이션 모델입니다.

---

## Desk

**Desk**는 독립적인 actor입니다. 이벤트를 받고, 한 가지 작업을 수행하고, 이벤트를 emit합니다.

```yaml
kind: desk
name: reviewer
executor:
  type: api          # 실행 방식
agent: senior-reviewer
```

Executor 타입:
- **api** — LLM 호출 (Anthropic, OpenAI, Gemini)
- **exec** — 임의 프로세스 (스크립트, CLI 도구, Python)
- **docker** — 컨테이너화된 프로세스
- **human** — 알림을 보내고 사람의 응답을 기다림
- **remote** — 다른 머신의 워커

사람도 `type: human`인 desk에 불과합니다. 별도의 특별한 개념이 필요하지 않습니다.

---

## Agent

**Agent**는 재사용 가능한 역할입니다. desk에 앉는 *사람*이 누구인지 — 무엇을 알고, 어떻게 생각하는지 — 를 정의합니다.

```yaml
kind: agent
name: senior-reviewer
skills:
  - code-review
  - security-checklist
```

한 번 정의하고 여러 desk에서 사용합니다. 역할이 한 곳에서만 쓰인다면 인라인으로 정의해도 됩니다.

---

## Skill

**Skill**은 재사용 가능한 지식 조각입니다 — 프롬프트, 가이드라인, 학습된 패턴.

```
skills/
  code-review.md        # 코드를 리뷰하는 방법
  security-checklist.md # 무엇을 확인해야 하는지

knowhow/
  common-bugs.md        # 시간이 지나며 쌓인 패턴
```

Skill은 agent로 조합됩니다. Knowhow는 시스템이 학습하면서 축적됩니다.

---

## Group

**Group**은 팀입니다. Group 안의 desk는 이벤트 스트림을 공유합니다 — 모든 desk가 다른 desk의 결과물을 볼 수 있습니다.

```yaml
kind: group
name: dev-team
lead: implementer      # 조율자: 먼저 계획하고, 마지막에 종합

desks:
  - implementer-b
  - reviewer
  - tester
```

Lead desk는 두 번 실행됩니다: 처음에는 계획하고 작업을 배분하고, 마지막에는 결과를 종합합니다. 중간에 멤버들이 작업합니다.

```
lead (계획)  →  members (작업)  →  lead (종합)  →  output event
```

---

## Resource

**Resource**는 외부 시스템과의 연결입니다. 두 가지 면이 있습니다:

**Watch** — 무언가 변경될 때 이벤트를 emit합니다 (GitHub PR이 열리거나, Figma가 업데이트되거나, 메시지가 수신되거나).

**Actions** — desk가 스크립트나 skill로 구현된 작업을 요청할 수 있습니다.

```yaml
kind: resource
name: codebase
type: github
repo: acme/app

watch:
  - pull_request
  - issue

actions:
  commit:
    exec: scripts/commit.sh
  notify:
    exec: scripts/slack.sh

permissions:
  patcher:   [commit]
  reporter:  [notify]
```

Resource는 완전히 커스텀으로 만들 수 있습니다 — 임의의 스크립트로 자신만의 이벤트와 액션을 정의하세요. 내장 타입(github, figma, slack)은 미리 패키징된 resource에 불과합니다.

---

## Organization

**Organization**은 전체 시스템입니다 — 항상 실행 중이고, 항상 준비된 상태입니다.

어떤 group이 존재하는지, 어떤 resource에 연결되는지, 이벤트가 group 사이에서 어떻게 라우팅되는지를 정의합니다.

```yaml
kind: organization
name: engineering

groups:
  - strategy-team
  - dev-team
  - ops-team

routing:
  - on: task.created   → strategy-team
  - on: plan.ready     → dev-team
  - on: code.ready     → ops-team
```

작업이 이벤트로 들어오면, organization이 적절한 group으로 라우팅합니다. Group이 처리합니다. task마다 수동으로 연결할 필요가 없습니다.

---

## Session

**Session**은 메모리입니다.

**Desk session** — 각 desk의 고유한 히스토리. 실행 간에 지속됩니다. 리뷰어는 지난번에 무엇을 지적했는지 기억합니다.

**Group session** — group 실행 내의 공유 컨텍스트. 모든 desk가 서로의 작업을 볼 수 있습니다. 매 실행마다 초기화됩니다.

---

## Policy

**Policy**는 desk나 group에 부착하는 규칙 집합입니다.

```yaml
kind: policy
name: careful
retry: 3
timeout: 5m
cost_limit: $0.10
```

실패 시 재시도, 막히면 타임아웃, 비용 상한 설정. 어떤 desk나 group에도 부착할 수 있습니다.

---

## 조합 방식

```
Skill + Skill           → Agent
Agent + Executor        → Desk
Desk + Desk + Resource  → Group
Group + Group + Routing → Organization
```

작은 팀의 예시:

```
resource: github-repo (watch: pull_request)
      ↓ event: pr.opened
organization routes → dev-team
      ↓
lead desk plans → members review + test → lead synthesizes
      ↓ event: review.done
resource action: github-repo.notify
```

이것이 전체 모델입니다.

---

## 현재 구현 상태

| 개념 | 상태 |
|---|---|
| Event (범용 메시지, 이벤트 버스) | ✅ |
| Desk (api, exec, docker, human, remote) | ✅ |
| Agent + Skill | ✅ |
| Group with coordinator pattern (both/first/last) | ✅ |
| Resource (타입 정의 + watch 플레이스홀더) | ✅ |
| Organization (group/desk 간 이벤트 라우팅) | ✅ |
| Session (desk + group) | ✅ |
| Policy (타입 정의) | ✅ |
| Actor model 이벤트 버스 | ✅ |
| Resource watcher (실제 polling/webhook) | 🔲 |
| Policy 집행 (retry, timeout, cost) | 🔲 |
