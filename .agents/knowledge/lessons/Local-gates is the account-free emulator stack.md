---
type: lesson
title: Local-gates is the account-free emulator stack
description: make local-gates runs Pulumi mock graphs, the offline acceptance harness, Floci AWS, and floci-gcp. Live clouds are only for E2E certify; GCP is thorough, other providers are light smoke.
tags: [testing, floci, pulumi, acceptance, spend]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-19
---

# Local-gates is the account-free emulator stack

Default proof order for MageLift:

1. Package tests and Pulumi mocks (`make pulumi-mock-test` / `go test ./internal/cloud/...`)
2. Offline harness dry-runs and shape checks (`make acceptance-harness-test`)
3. Floci AWS (`make floci-test-aws`) and floci-gcp (`make floci-gcp-test`)

`make local-gates` runs 1–3. `make verify` stays the contributor lint/unit/docs gate and does not start Docker emulators.

Live AWS, OVH, Scaleway, Cloudflare, New Relic, and Fastly exist only when an E2E cell is still required to certify. Those accounts run on free credits or free plans: one bounded smoke, then destroy. GCP has unlimited credits and is the thorough Magento-wired live path. Emulators never certify Autopilot, Memorystore, Armor, managed TLS, or Magento Cloud SQL PITR.

The AWS Floci Make target is `floci-test-aws` (script `scripts/floci-test-aws.sh`), matching `floci-gcp-test`. Do not run `go test ./tests/floci` without `-tags=floci`.
