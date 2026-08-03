---
type: lesson
title: MageLift CLI command registration must test duplicate names
description: When lifecycle commands replace reserved placeholders, remove the placeholder from commandGroups
  or Cobra exposes duplicate top-level commands and help output becomes ambiguous.
tags:
- lesson
- cli
- cobra
- testing
generated:
  at: '2026-07-24'
---

When lifecycle commands replace reserved placeholders, remove the placeholder from commandGroups or Cobra exposes duplicate top-level commands and help output becomes ambiguous. A root command test now asserts unique command names.
