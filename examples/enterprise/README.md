# Enterprise Example — MegaCorp

> **This example represents the desired end-state of a large enterprise organization.**
> Running all 29 desks will incur API costs per call.
> Use this primarily for dashboard visualization and org structure demos.

## Overview

- **5 divisions**: Product, Engineering, Data, Operations, Business
- **12 groups**: team-level units within each division
- **29 desks**: covering every role from PM to customer support
- **6 resources**: GitHub, Jira, Datadog, Database, Notion, Slack
- **22 routing rules**: cross-division event-driven workflows

## Organization Structure

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
