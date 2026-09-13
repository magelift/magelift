---
type: lesson
title: Docker Bake map matrix exposes primitive values
description: MageLift docker-bake.hcl used `php.key` and `php.value` for a map matrix, but Buildx exposed
  each matrix item as the primitive map value.
tags:
- docker
- buildx
- bake
status: stable
generated:
  at: '2026-07-24'
---

MageLift docker-bake.hcl used `php.key` and `php.value` for a map matrix, but Buildx exposed each matrix item as the primitive map value. Use a list of objects with explicit branch and base fields when both the label and value are needed.
