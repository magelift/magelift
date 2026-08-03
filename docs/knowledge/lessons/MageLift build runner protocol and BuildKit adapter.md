---
type: lesson
title: MageLift build runner protocol and BuildKit adapter
description: MageLift build runner protocol v1 is byte-compatible across Go and PHP, strict, bounded to
  1 MiB, canonical, and has separate prepare/finalize stages.
tags:
- phase-2
- protocol
- buildkit
- security
- build
generated:
  at: '2026-07-24'
---

MageLift build runner protocol v1 is byte-compatible across Go and PHP, strict, bounded to 1 MiB, canonical, and has separate prepare/finalize stages. The concrete PHP runner copies a clean source into private rootfs, excludes control state, rejects auth.json and env.php, runs the Magento lifecycle, and writes external digest-bound manifests. The Go pipeline uses isolated Docker prepare/finalize, BuildKit packaging, local runtime identity checks, manifest verification, and atomic publication. A real magelift build command and Docker fixture E2E pass. Release pushes, registry signing, credential-reference resolution, and SLSA publication remain Phase 2 work.
