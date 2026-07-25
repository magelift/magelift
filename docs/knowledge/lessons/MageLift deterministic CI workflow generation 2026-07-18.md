---
type: lesson
title: MageLift deterministic CI workflow generation 2026-07-18
description: ci generate validates every configured environment, requires an immutable MageLift release
  version, atomically writes .github/workflows/magelift.yml, and pins checkout v7 and setup-go v7 by commit
  ...
tags:
- cli
- ci
- github-actions
generated:
  at: '2026-07-24'
---

ci generate validates every configured environment, requires an immutable MageLift release version, atomically writes .github/workflows/magelift.yml, and pins checkout v7 and setup-go v7 by commit SHA. ci validate regenerates expected bytes and returns exit code 2 for missing or drifted workflows.
