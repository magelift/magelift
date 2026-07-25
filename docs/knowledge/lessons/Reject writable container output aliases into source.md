---
type: lesson
title: Reject writable container output aliases into source
description: The isolated build runner must resolve source and output paths through symlinks, then reject
  equality and ancestor or descendant overlap.
tags:
- security
- containers
- build-runner
generated:
  at: '2026-07-24'
---

The isolated build runner must resolve source and output paths through symlinks, then reject equality and ancestor or descendant overlap. Otherwise a writable /output bind can alias the read-only /workspace tree and defeat source isolation. Run the container as the invoking host UID/GID so a private 0700 host output remains writable without weakening permissions; mount a hardened private /tmp and set HOME there.
