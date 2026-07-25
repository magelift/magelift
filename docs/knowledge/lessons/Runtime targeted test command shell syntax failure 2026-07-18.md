---
type: lesson
title: Runtime targeted test command shell syntax failure 2026-07-18
description: A targeted go test command used an unquoted pipe in the -run argument, so zsh interpreted
  test names as commands.
tags:
- testing
- shell
- verification
generated:
  at: '2026-07-24'
---

A targeted go test command used an unquoted pipe in the -run argument, so zsh interpreted test names as commands. Future test regexes must be quoted or use separate -run invocations.
