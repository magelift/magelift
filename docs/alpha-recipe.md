---
title: Archived GCP alpha recipe
description: Versions, infrastructure and limits of the archived GCP staging run.
---

# Archived GCP alpha recipe

The alpha campaign exercised the GCP Autopilot configuration below. For a new shop, start with the current project template and choose versions for your Magento release. [Get started](getting-started.md)

## Pins

| Layer | Pin |
| --- | --- |
| Magento | Open Source 2.4.9 |
| PHP | 8.5 |
| Runtime | GCP GKE Autopilot, `europe-west1` |
| Database | Cloud SQL MySQL 8.4, `db-perf-optimized-N-2` |
| Cache/session | Memorystore Valkey 9 (`VALKEY_9_0`), `SHARED_CORE_NANO`, 1 shard |
| Search | OpenSearch 3 on GKE, digest-pinned image, recipe replica count |
| Queue | Database queue (no broker on preview) |
| Media | Private GCS bucket (fine-grained, HMAC); delivery through the storefront via get.php; `mediaURL` output is the store media base |
| Edge | HTTPS load balancer, Google-managed TLS |
| Web | 1 replica (preview preset) |
| Mail | Operator SMTP relay, password in Secret Manager (or disabled) |

Use `magelift --env staging config effective` to inspect the current GCP starter. [GCP staging walkthrough](onboarding.md)

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
