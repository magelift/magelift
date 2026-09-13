## Context

Scaleway is a typed `ScalewayTarget` with Kapsule, RDB, Redis (`cacheMode: redis`), and Cockpit. Bootstrap is `ErrNotSupported`. Live bar is one Kapsule preview from a **$50** own-money cap shared with vendors. Cache family is Redis, not Valkey — Adobe intersection must stay visible.

## Goals / Non-Goals

**Goals:**
- One Scaleway-owned spec and matrix section for every implemented cell.
- Certified subset stays empty; experimental stays explicit.
- One preview inside the $50 cap; destroy always; no KEEP.
- Campaign 6.4 Scaleway moves here. Vendors stay on GCP origin.

**Non-Goals:**
- KEEP, certified Kapsule, or implementing Bootstrap in this change.
- Treating Cockpit sources or account-free cost as Magento evidence.
- Cartesian live shops.

## Decisions

1. **Catalog = `ScalewayTarget` fields.** RDB HA/backup/encryption and Redis cluster size 1–6 are the field API.

2. **Money cap is shared.** A Scaleway preview competes with Cloudflare/SendGrid/Fastly/New Relic. If the cap is exhausted, the cell stays `not-run` and visible.

3. **Redis honesty.** Do not relabel Scaleway Redis as Valkey in the matrix.

**Alternatives considered:** Attach vendors to Scaleway origin (rejected until this spec explicitly allows it). Skip live preview entirely (allowed if $50 is spent elsewhere; catalog still required).

## Risks / Trade-offs

- $50 may buy zero Magento on Kapsule if vendors spend first. Catalog still lists implemented cells as experimental/`not-run`.
- Redis vs Adobe Valkey requirement can block “Adobe-ok” even when MageLift implements the topology. That mismatch MUST stay in the matrix, not be papered over.
