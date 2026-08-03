---
type: lesson
title: Vet shared dependencies separately during parallel runtime edits
description: A combined vet of bootstrap and CLI failed while another agent was splitting AWS runtime
  helpers, leaving temporary undefined symbols.
tags:
- go
- vet
- shared-worktree
status: stable
generated:
  at: '2026-07-24'
---

A combined vet of bootstrap and CLI failed while another agent was splitting AWS runtime helpers, leaving temporary undefined symbols. The targeted bootstrap and CLI race tests passed. Verify the owned bootstrap package independently and leave runtime integration verification to its owner.
