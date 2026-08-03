---
type: lesson
title: 'MageLift FrankenPHP matrix build failure: LABEL continuation 2026-07-18'
description: The first FrankenPHP classic matrix Docker build failed because an apply_patch-generated
  Dockerfile LABEL used two literal backslashes for line continuations.
tags:
- docker
- frankenphp
- buildkit
- debugging
generated:
  at: '2026-07-24'
---

The first FrankenPHP classic matrix Docker build failed because an apply_patch-generated Dockerfile LABEL used two literal backslashes for line continuations. BuildKit then parsed the continuation as an invalid LABEL token and reported an undefined variable warning. The fix was to use one Dockerfile continuation backslash per line and redeclare FRANKENPHP_BASE after FROM so the base reference is available to labels. The 8.2-8.5 amd64 matrix then built successfully with SBOM and provenance attestations.
