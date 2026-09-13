---
type: lesson
title: floci-gcp 0.7.0 is the GCP offline emulator pin
description: Pin floci/floci-gcp:0.7.0 by digest for GCS, Secret Manager, Pub/Sub, Logging, and Monitoring. It does not certify Autopilot, Memorystore, Armor, managed TLS, or Magento Cloud SQL PITR.
tags:
- floci
- gcp
- testing
status: stable
stale_after: 2027-02-18
generated:
  by: cursor-grok-4.6/darwin
  at: '2026-08-18'
---

Digest-pinned `floci/floci-gcp:0.7.0` (`make floci-gcp-test`, CI job `floci-gcp`) is the account-free GCP API layer. GCS, Secret Manager create/access, Pub/Sub topic publish, Cloud Logging write/list, and Cloud Monitoring time-series write/list are covered with official Go clients. The Go Secret Manager, Logging, and Monitoring GAPIC clients do not honor `*_EMULATOR_HOST`; tests dial `:4588` with plaintext gRPC. Floci AWS is pinned at `1.7.0`. Emulators never certify GKE Autopilot, Memorystore Valkey 9.0, Cloud Armor data plane, Google-managed TLS, or Magento Cloud SQL PITR. Those stay on packed live sessions (`docs/certification-sessions.md`).
