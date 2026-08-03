---
type: lesson
title: MageLift production deployment re-verifies release signatures 2026-07-18
description: A production deploy must not trust release-journal signature metadata alone.
tags:
- magelift
- release
- cosign
- production
- security
generated:
  at: '2026-07-24'
---

A production deploy must not trust release-journal signature metadata alone. requireSignedRelease now calls verifyRelease for the exact digest using the recorded certificate identity and OIDC issuer, and fails closed when verification is unavailable or invalid. Tests cover both paths.
