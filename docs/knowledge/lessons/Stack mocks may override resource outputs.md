---
type: lesson
title: Stack mocks may override resource outputs
description: The runtime stack wiring test initially expected the configured media bucket name, but the
  shared stack mock deliberately overrides every S3 bucket output to shop-media.
tags:
- pulumi
- mocks
- stack
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

The runtime stack wiring test initially expected the configured media bucket name, but the shared stack mock deliberately overrides every S3 bucket output to shop-media. Assertions on composed outputs must use mock state, not the original resource input.
