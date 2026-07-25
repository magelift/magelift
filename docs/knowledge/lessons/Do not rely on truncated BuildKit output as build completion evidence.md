---
type: lesson
title: Do not rely on truncated BuildKit output as build completion evidence
description: A long docker buildx bake invocation exceeded tool output limits.
tags:
- docker
- buildx
- verification
- failed-attempt
generated:
  at: '2026-07-24'
---

A long docker buildx bake invocation exceeded tool output limits. The wait result appeared complete, but no images were loaded, so the build had failed after the visible portion. Re-run cached builds with quiet progress and inspect the expected image tags before claiming success.
