---
type: lesson
title: Live cells stay inside AWS and OVH $200 caps
description: AWS and OVH live acceptance must stay within ~$200 credits each; Scaleway is free-plan only; GCP spend is ignored.
tags: [acceptance, billing, aws, ovh, scaleway]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
verified:
  - by: human:alex
    at: 2026-08-14
---

# Live cells stay inside AWS and OVH $200 caps

Operator rule for MageLift live cells (2026-08-14):

| Provider | Spend rule |
| --- | --- |
| AWS | Stay inside ~$200 free credits. Use Budgets **actual**, not Cost Explorer unblended (credits mask it to ~$0). Budgets: `magelift-5-usd` (80% already ALARM at $4.95), `magelift-25-usd`, and `magelift-200-usd` (75%/90% → SNS `magelift-budget-alerts`). |
| OVH | Stay inside ~$200 credits. Alert `magelift-150` id `a9145495-5ee8-4527-815b-1f843959c0da` fires at **€150**/month (`service=all`, delay 3600s). OVH thresholds are EUR, not USD. Alerts do not stop resources. |
| Scaleway | Free plan: **no incurred cost**. Do not create paid SKUs (Kapsule nodes, RDB, Redis, LB, Instances). If a Magento/runtime cell needs those, record it blocked/unsupported instead of billing. |
| GCP | Ignore spend. The operator is not paying there. |

Keep only the resources required to finish the current certification cell. One live mutation per paying provider. Destroy before the next paid cell. Do not set `KEEP=true`. Do not start HA/DR while cheaper architecture baselines are still open.

AWS Budgets actual on 2026-08-14 was $4.95 MTD. Added `magelift-200-usd` (75%/90%) and OVH alert `magelift-150` at €150. Independent inventories that day: no leftover OVH DB/instance/MKS/LB; no leftover Scaleway Kapsule/RDB/Redis/instance/LB.
