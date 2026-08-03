---
type: lesson
title: Live cost command compile failure 2026-07-18
description: The first live cost command edit used a named Cobra command parameter with an unnamed second
  parameter, which is invalid Go syntax.
tags:
- go
- cost
- verification
generated:
  at: '2026-07-24'
---

The first live cost command edit used a named Cobra command parameter with an unnamed second parameter, which is invalid Go syntax. Correct function signatures must name both parameters or neither before rerunning tests.
