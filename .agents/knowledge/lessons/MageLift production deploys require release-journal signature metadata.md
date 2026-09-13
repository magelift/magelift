---
type: lesson
title: MageLift production deploys require release-journal signature metadata
description: The CLI now refuses a production deploy unless the exact planned image digest appears in
  the local release journal with non-empty verified signature identity and issuer metadata.
tags:
- deployment
- supply-chain
- rollback
- cosign
generated:
  at: '2026-07-24'
---

The CLI now refuses a production deploy unless the exact planned image digest appears in the local release journal with non-empty verified signature identity and issuer metadata. Rollback re-verifies the selected digest and runs the same deployment workflow before recording the rollback event.
