---
type: lesson
title: Re-read shared root registration immediately before patching
description: A release CLI patch failed because internal/cli/root.go had changed since the earlier read
  in the shared multi-agent worktree.
tags:
- multi-agent
- apply-patch
- cli
- tool-failure
status: stable
generated:
  at: '2026-07-24'
---

A release CLI patch failed because internal/cli/root.go had changed since the earlier read in the shared multi-agent worktree. For shared registration files, inspect the current imports, options, and commandGroups immediately before applying a patch, then split new-file creation from the narrow registration edit.
