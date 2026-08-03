---
type: lesson
title: Re-read shared files after delegated edits before patching
description: On 2026-07-18, a combined apply_patch against the container runner failed verification even
  though an earlier read appeared to match.
tags:
- workflow
- apply-patch
- shared-worktree
generated:
  at: '2026-07-24'
---

On 2026-07-18, a combined apply_patch against the container runner failed verification even though an earlier read appeared to match. In a shared worktree, split cross-file patches and re-read the exact current region before retrying; do not repeat a broad stale patch.
