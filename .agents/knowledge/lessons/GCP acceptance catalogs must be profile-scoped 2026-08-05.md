---
type: lesson
title: GCP acceptance catalogs must be profile-scoped
description: Pair MAGELIFT_GCP_ACCEPTANCE_CELL_CATALOG with the matching PROFILE; cells-gcp-standard.txt plus PROFILE=preview fails queue:health.
tags:
- gcp
- acceptance
- certification
- testing
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
---

The standard and high-availability dry runs silently used the preview catalog
because the harness only read `MAGELIFT_ACCEPTANCE_CELL_CATALOG`. The profile
commands supplied `MAGELIFT_GCP_ACCEPTANCE_CELL_CATALOG`, which the script did
not read yet. The command still exited successfully, so the mistake was easy
to miss.

The GCP-specific variable now has precedence, with the generic variable as a
fallback. That is not enough: `queue:health` only accepts PROFILE `standard`
or `high-availability` (`expected=1` / `expected=2` RabbitMQ replicas). The
2026-08-13 Magento cell `20260813am` set `RUNTIME=gke-standard` and
`cells-gcp-standard.txt` while leaving PROFILE=`preview`. Magento
`deploy:candidate` passed; `queue:health` then failed with `requires standard
or high-availability profile` and EXIT destroyed the stack. Do not reuse `am`.
Retry GKE Standard Magento with `MAGELIFT_GCP_ACCEPTANCE_PROFILE=standard`,
`MAGELIFT_GCP_ACCEPTANCE_ALLOW_COSTLY=true`, and the standard catalog.

Each provider harness should print the resolved catalog and profile before
running cells. A successful Magento deploy is not a Standard catalog PASS
when a later profile-gated cell fails.
