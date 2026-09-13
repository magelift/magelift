# ADR 0010: Packed live certification, no paid multi-cloud CI

- Status: Accepted
- Date: 2026-08-22

## Context

Live Magento create/destroy is slow and expensive. Per-checkbox destroy burns hours without extra proof. GitHub cannot fund a full multi-cloud matrix.

## Decision

- Daily CI is account-free: Pulumi mocks, Floci AWS, floci-gcp where the emulator covers the API. Emulators do not certify Autopilot, Memorystore, Cloud Armor, managed TLS, Magento Cloud SQL PITR, or regional DR.
- Live certification uses packed KEEP sessions, not one stack per checkbox. GCP is the thorough Magento path. AWS, OVH, Scaleway, Cloudflare, New Relic, SendGrid, and Fastly are light smoke: one bounded cell, then destroy.
- Destroy on exit. Do not set `MAGELIFT_*_ACCEPTANCE_KEEP=true` unless the operator wants a retained debug cell.
- No paid multi-cloud GitHub matrix by default. Sparse maintainer runs on acceptance accounts only.

## Consequences

Evidence on the site is the current proof pack (`docs/evidence/README.md`), not a KEEP diary. Experimental Magento on OVH/Scaleway stays unpurchased on free plans that cannot run it.

## Alternatives considered

- Per-cell create/destroy: rejected (teardown cost, no extra proof).
- Certified Magento on four clouds before rc.1: rejected (time and SKU limits).

## Provenance

Maintainer acceptance practice. Floci public emulator docs.
