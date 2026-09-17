---
title: Alpha recipe
description: The single pinned GCP Autopilot shop shape the alpha proves. Versions, tiers, bounds, and excluded promises.
---

# Alpha recipe

The one shop shape the alpha proves. Everything here is a pin, not a
range. The acceptance loop runs exactly this; pilots should start here
before varying anything.

## Pins

| Layer | Pin |
| --- | --- |
| Magento | Open Source 2.4.9 |
| PHP | 8.5 |
| Runtime | GCP GKE Autopilot, `europe-west1` (the proved region) |
| Database | Cloud SQL MySQL 8.4, `db-perf-optimized-N-2` |
| Cache/session | Memorystore Valkey 9 (`VALKEY_9_0`), `SHARED_CORE_NANO`, 1 shard |
| Search | OpenSearch 3 on GKE, digest-pinned image, recipe replica count |
| Queue | Database queue (no broker on preview) |
| Media | GCS bucket from stack outputs |
| Edge | HTTPS load balancer, Google-managed TLS |
| Web | 1 replica (preview preset) |
| Mail | Operator SMTP relay, password in Secret Manager (or disabled) |

`magelift init --provider gcp` templates this shape; the loop and the
onboarding guide verify it per surface.

## Bounds

- Single region, preview preset for the loop. Staging and production
  presets size up deliberately and are not part of the alpha proof.
- One Magento/PHP/data-service combination. Other combinations are
  unproved, not forbidden; file results back.
- Compatible schema changes only. Incompatible migrations stay
  operator-gated and out of the loop.

## Excluded promises

No arbitrary versions, no zero-downtime incompatible schema changes,
no cross-cloud disaster recovery, no spending cap. Budgets alert;
amounts come from provider billing.

## Evidence

The loop's proof file and sealed run live under
`docs/evidence/` (see the acceptance report). Reuse them within
their scope; rerun when code, packaging, or the public execution
path changes.
