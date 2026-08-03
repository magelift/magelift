---
type: lesson
title: Inspect generated documentation paths before assertions
description: On 2026-07-17, a validation command guessed docs/reference/configuration.md and stopped before
  later checks.
tags:
- workflow
- generated-docs
- failure
generated:
  at: '2026-07-24'
---

On 2026-07-17, a validation command guessed docs/reference/configuration.md and stopped before later checks. Generated documentation paths must be discovered from the repository or generator metadata before asserting their contents.
