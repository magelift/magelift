---
type: lesson
title: MageLift shell glob probe failed 2026-07-18
description: A shell audit command used an unmatched Dockerfile* glob and zsh aborted before rg ran.
tags:
- magelift
- workflow
- shell
- failure
generated:
  at: '2026-07-24'
---

A shell audit command used an unmatched Dockerfile* glob and zsh aborted before rg ran. Use rg --files plus an explicit images path when probing files in this repository; unmatched globs are not evidence of missing files.
