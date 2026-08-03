---
type: lesson
title: Pulumi mock output assertions need synchronization
description: The security component race test found concurrent writes to the test output map from multiple
  Pulumi ApplyT callbacks.
tags:
- pulumi
- go
- race-test
- failed-attempt
status: stable
generated:
  at: '2026-07-24'
---

The security component race test found concurrent writes to the test output map from multiple Pulumi ApplyT callbacks. Pulumi output callbacks may run concurrently, so tests that collect resolved outputs must guard shared state with a mutex or use independent channels.
