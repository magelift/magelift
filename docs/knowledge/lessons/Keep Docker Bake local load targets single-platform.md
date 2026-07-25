---
type: lesson
title: Keep Docker Bake local load targets single-platform
description: MageLift initially put amd64 and arm64 on the base Bake target, then used --load for local
  verification.
tags:
- docker
- buildx
- multiarch
status: stable
generated:
  at: '2026-07-24'
---

MageLift initially put amd64 and arm64 on the base Bake target, then used --load for local verification. Docker local loading is single-platform, so keep the local target native-platform and assign the multi-platform list only to the registry matrix target.
