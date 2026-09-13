---
type: lesson
title: Cleanup ledger run IDs must be unique across providers
description: Reconcile scans the whole `.magelift/cleanup` directory; reusing a run ID or duplicating claimed identities blocks the next provider cell before it mutates.
tags: [cleanup, ledger, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
---

`acceptance_cleanup_reconcile_pending` compiles `magelift cleanup reconcile`
against every JSON ledger in the directory. CloudFront `20260813n` claimed the
same ACM ARN twice after a retry. OVH then reused `20260813n` and exited
before create.

Give every live cell a unique run ID. After verified empty inventory, mark
ledger rows `gone` (or remove the file) so the next cell is not blocked by a
finished provider.
