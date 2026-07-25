---
type: lesson
title: MageLift real AWS acceptance matrix 2026-07-18
description: Added a spend-gated .github/workflows/aws-integration.yml and reusable scripts/aws-integration.sh.
tags:
- aws
- integration
- github-actions
- oidc
- verification
status: deprecated
generated:
  at: '2026-07-24'
---

Added a spend-gated .github/workflows/aws-integration.yml and reusable scripts/aws-integration.sh. The workflow uses pinned checkout v7, setup-go v7, and configure-aws-credentials v5 with GitHub OIDC, matrix profiles preview/standard/high-availability, per-profile concurrency and environment approvals, a base64 configuration secret, signed digest verification through promote, deploy, runtime health, outputs, and exit-trap cleanup. It remains disabled until MAGELIFT_AWS_INTEGRATION_ENABLED is true and the role/configuration/digest variables are reviewed.
