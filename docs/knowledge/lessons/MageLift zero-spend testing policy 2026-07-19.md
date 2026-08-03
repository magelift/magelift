---
type: lesson
title: MageLift zero-spend testing policy 2026-07-19
description: 'Product constraint: maintainer cannot fund ongoing AWS spend.'
tags:
- testing
- floci
- aws
- budget
- product
status: stable
generated:
  at: '2026-07-24'
---

Product constraint: maintainer cannot fund ongoing AWS spend. Offline Floci is the primary automated verification path. Real AWS (and later GCP/Azure/OVH/Hetzner) runs are sparse, destroy-immediately, minimize uptime, and are not a continuous CI matrix. Public posture must not claim real-cloud certification; community reports from disposable customer accounts are the main real-provider feedback loop after pre-alpha. Keep aws-integration.yml spend-gated and default-disabled. Prefer Floci over Ministack for offline coverage plus future multi-cloud emulators. Before any production-readiness or public-stable claim, require at least one documented sparse preview bootstrap-deploy-destroy on real AWS or an explicit unsupported-community-validated label.
