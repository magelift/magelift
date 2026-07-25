---
type: lesson
title: MageLift migration timeout test expected wrong error text 2026-07-18
description: The ECS migration timeout path returns a wrapped context deadline exceeded error after stopping
  the task; the injected test initially expected the StopTask reason text.
tags:
- testing
- error
status: stable
generated:
  at: '2026-07-24'
---

The ECS migration timeout path returns a wrapped context deadline exceeded error after stopping the task; the injected test initially expected the StopTask reason text. Assert timeout semantics and stopped-task evidence instead of an implementation detail.
