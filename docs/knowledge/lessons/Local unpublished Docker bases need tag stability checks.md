---
type: lesson
title: Local unpublished Docker bases need tag stability checks
description: Even the Docker-backed default buildx builder attempted to pull a digest-qualified magelift/php-runtime
  repository that existed only in the local daemon.
tags:
- docker
- buildkit
- local-development
- supply-chain
generated:
  at: '2026-07-24'
---

Even the Docker-backed default buildx builder attempted to pull a digest-qualified magelift/php-runtime repository that existed only in the local daemon. For unpublished local development images, use the local tag and verify its immutable image ID immediately before and after the application build; fail if it changes. Release builds must continue to use a registry-qualified digest and do not use this local exception.
