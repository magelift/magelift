---
type: lesson
title: Preserve safe runtime environment variables for isolated Composer
description: The second build E2E failed in Composer validation because NativePreparation replaced the
  child environment with only PATH.
tags:
- php
- composer
- containers
- diagnostics
generated:
  at: '2026-07-24'
---

The second build E2E failed in Composer validation because NativePreparation replaced the child environment with only PATH. The container set HOME=/tmp, but proc_open did not pass it through. Isolated child processes should use an explicit safe allowlist that includes PATH and HOME; Composer authentication remains a separate per-command secret variable. Lifecycle errors should identify the phase, executable, and exit code without echoing potentially sensitive output.
