---
type: lesson
title: Preserve injected CLI option defaults
description: newCommandWithOptions binds Cobra persistent flags directly to injected option pointers.
tags:
- go
- cobra
- testing
- cli
generated:
  at: '2026-07-24'
---

newCommandWithOptions binds Cobra persistent flags directly to injected option pointers. Cobra applies each flag default during registration, so test-only config paths, environments, and output formats were overwritten when callers omitted those flags. Use the existing option value as each flag default, falling back to the normal CLI default only when it is empty.
