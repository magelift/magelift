---
type: lesson
title: MageLift GoReleaser check requires git remote 2026-07-18
description: GoReleaser v2.17.0 container was available, but goreleaser check failed in this fresh workspace
  because git has no configured remote; CI with github origin is the authoritative validation.
tags:
- goreleaser
- verification
- environment
generated:
  at: '2026-07-24'
---

GoReleaser v2.17.0 container was available, but goreleaser check failed in this fresh workspace because git has no configured remote; CI with github origin is the authoritative validation. The configuration itself was not changed to hide this local precondition.
