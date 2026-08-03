---
type: lesson
title: MageLift local AWS acceptance replaces CI matrix 2026-07-19
description: Removed .github/workflows/aws-integration.yml.
tags:
- aws
- acceptance
- adr-0007
- testing
status: deprecated
generated:
  at: '2026-07-24'
---

Removed .github/workflows/aws-integration.yml. Real AWS acceptance is local via scripts/aws-acceptance-local.sh and docs/aws-acceptance.md: preview default, destroy on EXIT, costly profiles require MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY. ADR 0007 defines certified/experimental/community provider tiers; AWS v1, GCP v1.1 first-party candidate, later Azure/OVH/Hetzner as first-party or community sdk/v1 modules. No paid multi-cloud GitHub matrix by default.

# Related

* Supersedes: Projects/magelift/Lessons/MageLift real AWS acceptance matrix 2026-07-18
