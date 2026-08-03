---
type: lesson
title: Add context imports with injected CLI factories
description: On 2026-07-18 bootstrap CLI wiring introduced a context-based injected factory but omitted
  the root package context import, causing an immediate compile failure.
tags:
- go
- cli
- testing
- failure
generated:
  at: '2026-07-24'
---

On 2026-07-18 bootstrap CLI wiring introduced a context-based injected factory but omitted the root package context import, causing an immediate compile failure. When adding dependency-injection fields to command options, compile the owning package before writing tests.
