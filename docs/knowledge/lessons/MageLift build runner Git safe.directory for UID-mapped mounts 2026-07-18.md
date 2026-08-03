---
type: lesson
title: MageLift build runner Git safe.directory for UID-mapped mounts 2026-07-18
description: The build container runs as the invoking host UID on Unix so private bind-mounted output
  remains writable.
tags:
- build
- security
- docker
status: stable
generated:
  at: '2026-07-24'
---

The build container runs as the invoking host UID on Unix so private bind-mounted output remains writable. Git then sees the mounted source as owned by a different UID and refuses it as dubious ownership. GitRevisionVerifier must invoke git with -c safe.directory=<absolute repository root> before -C; this permits exact HEAD verification without changing source ownership or running the build as root.
