---
status: accepted
slug: operator-finops-catalog
---

# Intent: one command catalog for run, debug, and spend

## Problem

Teams leaving bare metal or PaaS need to answer three questions from the CLI: is it healthy, what broke, what does it cost. The commands exist but unevenly: health layers, log tailing, exec, cost live pricing (ECS only), budget reads, audit versus evidence, tunnel targets, and cleanup reconcile each have their own gaps and provider asymmetries. An operator should not need the cloud console for routine work on a certified target.

## Evidence

`magelift health` has config/outputs/runtime modes with `infrastructure/service/magento/dependency/deployment` layers; exit 4 unhealthy, 3 unavailable. `magelift logs --service web|deploy|cron` pages CloudWatch with filter/limit/since. `magelift exec`/`ssh` run over ECS Exec or kubectl; `--service deploy` rejected by design. `magelift cost` defaults account-free via `CostEstimator`; `--live` queries AWS Price List for ECS capacity only; transfer/requests/storage/logs/WAF/CloudFront/NAT stay unsupported. `cost --budget` reads AWS Budgets and GCP budgets with scoping limits; OVH/Scaleway report unavailable. `magelift evidence` (change journal) and `magelift audit` (control matrix) are distinct by contract. `tunnel` is capability-driven with explicit unsupported paths. `cleanup reconcile` deletes ledger-claimed leftovers. `pulumi-error-causes` intent covers deploy-failure classification. Which commands agencies actually reach for first: not checked.

## Proposed outcome

On certified targets, routine run/debug/spend work stays in the CLI: layered health that refuses to infer runtime state from config, logs plus exec for web/cron plus deploy log reads, cost inputs with live ECS prices and explicit unpriced lists, budget reads that never present a budget as a forecast or an enforcement, evidence and audit exports with no secret values, tunnels that fail explicitly rather than falling back to public endpoints, and cleanup reconcile that finishes interrupted runs. Provider gaps stay typed unsupported with the capability named.

## Affected users and systems

On-call developers, agency operators, cost reviewers. `magelift health/logs/exec/ssh/cost/evidence/audit/tunnel/cleanup`, provider Observe/Steps/State/Secrets ports, `docs/operations.md`, user skill `magelift-operate`.

## Constraints

No secret values in output, ever. A budget is never a forecast and never blocks deploys. Unpriced capacity stays listed as unpriced, never zero. Day-2 ports return `ErrNotSupported` until ready; no stub success. Live spend evidence needs provider billing reports or export, not CLI invention. Keep `pulumi-error-causes` in scope as the deploy-failure half of this catalog.

## Out of scope

Provider cost adapters beyond AWS ECS live pricing (GCP/OVH/Scaleway/EKS adapters are post-v1 per `docs/post-beta-roadmap.md`), FinOps SaaS (anomaly, rightsizing), Cloud SQL attach, cross-provider spend rollups.

## Open questions

Which five operator commands must be live-proved per certified origin for v1, and which ride on Floci plus unit proof? Do we add a single `status` dashboard combining health plus cost inputs, or keep the verbs separate?
