---
type: lesson
title: MageLift audit missing sdk interfaces file 2026-07-18
description: The SDK extension interfaces are in sdk/v1/types.go and topology.go; attempting to read a
  nonexistent sdk/v1/interfaces.go failed.
tags:
- workflow
- error
status: stable
generated:
  at: '2026-07-24'
---

The SDK extension interfaces are in sdk/v1/types.go and topology.go; attempting to read a nonexistent sdk/v1/interfaces.go failed. Use codebase discovery before direct file reads.
