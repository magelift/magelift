---
type: lesson
title: Run the compiler immediately after import edits
description: On 2026-07-18 the MageLift CLI build failed because a planned strings helper was not used.
tags:
- go
- compiler
- workflow
generated:
  at: '2026-07-24'
---

On 2026-07-18 the MageLift CLI build failed because a planned strings helper was not used. Run gofmt and a focused compile immediately after import-heavy patches, then remove abandoned imports before broader verification.
