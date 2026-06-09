# Enterprise Example — MegaCorp

> **이 예시는 최종적으로 원하는 대기업 규모 조직의 모습입니다.**
> 실제로 전부 실행하려면 29개 데스크 × API 호출 비용이 발생합니다.
> 대시보드 시각화와 조직 구조 데모 목적으로 사용하세요.

## Overview

- **5개 사업부**: Product, Engineering, Data, Operations, Business
- **12개 그룹**: 각 부서 내 팀 단위
- **29개 데스크**: PM부터 고객지원까지 전 직군
- **6개 리소스**: GitHub, Jira, Datadog, Database, Notion, Slack
- **22개 라우팅 룰**: 부서 간 이벤트 기반 협업 흐름

## 조직 구조

```
MegaCorp
├── Product Division
│   ├── product-strategy (product-manager, product-analyst)
│   └── design-team (ux-designer, ui-designer)
├── Engineering Division
│   ├── frontend-team (frontend-lead, frontend-dev, frontend-dev-2)
│   ├── backend-team (backend-lead, backend-dev, backend-dev-2, db-engineer)
│   ├── infra-team (cloud-engineer, network-engineer)
│   └── qa-team (qa-lead, qa-engineer, qa-automation)
├── Data Division
│   ├── analytics-team (data-analyst, bi-engineer)
│   └── ml-team (ml-engineer, data-scientist)
├── Operations Division
│   ├── devops-team (sre-engineer, devops-engineer)
│   └── security-team (security-analyst, compliance-officer)
└── Business Division
    ├── marketing-team (marketing-lead, content-writer, social-media)
    └── support-team (support-lead, support-agent)
```

## Event Flow

```
feature.requested → product-strategy → spec.approved → design-team
                                                          ↓
                                               design.completed
                                              ↙            ↘
                                    frontend-team      backend-team
                                         ↓                  ↓
                                  frontend.ready      backend.ready
                                         ↘                ↙
                                          qa-team
                                        ↙       ↘
                                 qa.passed    qa.failed → (back to eng)
                                    ↓
                               devops-team → deploy.ready → infra-team
                                    ↓                          ↓
                          release.published          deploy.completed
                            ↙         ↘              ↙            ↘
                    marketing    support      analytics      security
                                    ↓              ↓
                         customer.feedback    data.insight
                                    ↘              ↙
                              product-strategy (feedback loop)
```

## Running

```bash
# Validate
roster dry-run .

# Start with dashboard
roster hub --dir . --ui :8080

# Trigger the flow
roster emit feature.requested '{"title": "Add dark mode"}'
```
