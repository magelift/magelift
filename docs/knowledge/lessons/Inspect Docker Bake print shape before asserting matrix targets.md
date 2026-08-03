---
type: lesson
title: Inspect Docker Bake print shape before asserting matrix targets
description: On 2026-07-17, a jq assertion guessed that Bake --print emitted a php-runtime-8-5 target
  and failed.
tags:
- docker
- bake
- test
- failure
generated:
  at: '2026-07-24'
---

On 2026-07-17, a jq assertion guessed that Bake --print emitted a php-runtime-8-5 target and failed. Inspect the printed target keys and normalized attest representation before writing exact assertions for matrix output.
