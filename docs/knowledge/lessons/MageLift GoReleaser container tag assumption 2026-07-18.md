---
type: lesson
title: MageLift GoReleaser container tag assumption 2026-07-18
description: Attempted to validate .goreleaser.yaml with goreleaser/goreleaser:v2.17; Docker Hub has no
  v2.17 tag, so this validation path failed.
tags:
- goreleaser
- verification
- failed-attempt
generated:
  at: '2026-07-24'
---

Attempted to validate .goreleaser.yaml with goreleaser/goreleaser:v2.17; Docker Hub has no v2.17 tag, so this validation path failed. Use the pinned goreleaser action or an explicit existing container tag instead of assuming the tag exists.
