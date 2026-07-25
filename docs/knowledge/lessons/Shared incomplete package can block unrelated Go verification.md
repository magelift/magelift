---
type: lesson
title: Shared incomplete package can block unrelated Go verification
description: MageLift CLI verification failed twice because an unrelated in-progress AWS operations file
  imported cloudwatchlogs before its Go module dependency was added.
tags:
- go
- shared-worktree
- verification
status: stable
generated:
  at: '2026-07-24'
---

MageLift CLI verification failed twice because an unrelated in-progress AWS operations file imported cloudwatchlogs before its Go module dependency was added. Stop retrying the same package build. Do not mutate another agent's dependency scope merely to clear it. Coordinate with the owner, verify the independent release store package, and rerun CLI verification after the shared worktree settles.
