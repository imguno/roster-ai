# 퀵스타트: 첫 번째 AI 에이전트 파이프라인

10분 안에 아무것도 없는 상태에서 동작하는 AI 파이프라인을 만들어 봅니다.

---

## 사전 요구사항

- Go 1.22+
- Anthropic API 키 ([여기서 발급](https://console.anthropic.com))

---

## 1. Roster 설치

```bash
git clone https://github.com/roster-io/roster
cd roster/roster
go build -o roster ./cmd/roster
# 원하면 PATH에 추가하세요
mv roster /usr/local/bin/roster
```

---

## 2. API 키 설정

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

---

## 3. 새 organization 초기화

```bash
roster init my-org
cd my-org
```

다음과 같은 구조가 생성됩니다:

```
my-org/
├── organization.yaml   # 라우팅 규칙
├── agents/
│   └── developer.yaml  # 에이전트 정체성 + skill
├── desks/
│   └── developer.yaml  # 에이전트 실행 방식
├── groups/
│   └── dev-team.yaml   # desk 팀
├── skills/
│   └── coding.md       # 프롬프트 skill
└── policies/
    └── standard.yaml   # 재시도 + 타임아웃
```

---

## 4. executor 설정 확인

`roster init`이 이미 동작하는 API executor를 `desks/developer.yaml`에 생성해 두었습니다:

```yaml
kind: desk
name: developer

agent: developer

executor:
  type: api
  sdk: anthropic
  params:
    model: claude-haiku-4-5-20251001
    api_key: ${ANTHROPIC_API_KEY}

policy: standard
```

`${ANTHROPIC_API_KEY}` 플레이스홀더는 로드 시점에 환경 변수에서 자동으로 치환됩니다. 수동으로 수정할 필요가 없습니다.

---

## 5. 설정 유효성 검사

```bash
roster dry-run .
```

예상 출력:
```
  ✓ Config loaded: 1 desks, 1 groups, 0 resources, 1 policies
  ✓ Validation passed
  ✓ All skills resolved
  ✓ Dry-run complete
```

---

## 6. hub 시작

```bash
roster hub --ui :8080
```

hub가 시작되고 대시보드 URL이 출력됩니다. 브라우저에서 `http://localhost:8080`을 열어보세요.

---

## 7. 첫 번째 task 전송

두 번째 터미널에서:

```bash
roster emit task.created "Write a Go function that checks if a number is prime"
```

이벤트가 라우팅 규칙을 따라 흐릅니다:

```
task.created  →  dev-team (group)  →  developer (desk)  →  Claude API
```

---

## 8. 결과 확인

**대시보드**: `http://localhost:8080`을 열면 이벤트 로그에서 토큰 수, 소요 시간, 사용 모델을 확인할 수 있습니다.

**CLI**:
```bash
roster logs --follow
```

---

## 무슨 일이 일어난 건가요?

`organization.yaml`에 라우팅 규칙이 정의되어 있습니다:

```yaml
kind: organization
name: my-org

groups:
  - dev-team

routing:
  - on: task.created
    to: dev-team
```

`groups/dev-team.yaml`은 `task.created`를 구독하고 작업을 `developer` desk로 라우팅합니다:

```yaml
kind: group
name: dev-team

desks:
  - developer

subscribe:
  - task.created

emit:
  - task.completed
```

`task.created`를 emit하면, hub가 라우팅 규칙을 매칭하고 그룹을 활성화하여, developer desk가 코딩 skill 프롬프트와 task 설명을 함께 Claude에 전달합니다.

---

## 다음 단계

- **desk 추가** — 리뷰어, 테스터, 배포 담당자
- **팀 구성** — 여러 desk를 한 group에 넣고, lead desk를 추가해 조율
- **도구 연결** — `resource`를 정의해 desk에서 GitHub, Slack 또는 임의의 API에 접근
- **스케줄 실행** — desk나 group에 `cron: "0 9 * * *"`를 추가해 정해진 시간에 실행
- **사람 참여** — `executor: {type: human}`으로 승인 단계 추가

모든 설정 옵션은 [YAML 레퍼런스](yaml-schema.md)를 참고하세요.

---

## 템플릿

`roster init`에는 일반적인 사용 사례를 위한 세 가지 추가 템플릿이 포함되어 있습니다:

```bash
roster init my-org --template product-team     # architect + dev + review + ops
roster init my-org --template content-pipeline # researcher + writer + editor
roster init my-org --template code-review      # security + quality reviewers (parallel)
```
