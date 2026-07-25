---
type: lesson
title: MageLift verification shell wrapper failure 2026-07-18
description: A verification rerun wrapper used the zsh readonly variable name status and failed before
  reporting make verify.
tags:
- verification
- shell
- error
status: stable
generated:
  at: '2026-07-24'
---

A verification rerun wrapper used the zsh readonly variable name status and failed before reporting make verify. This was a shell wrapper error, not a repository test failure. Use rc or another variable name when capturing command results in zsh.
