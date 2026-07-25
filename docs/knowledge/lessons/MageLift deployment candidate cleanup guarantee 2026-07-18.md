---
type: lesson
title: MageLift deployment candidate cleanup guarantee 2026-07-18
description: The provider-neutral deployment orchestrator tracks successful candidate task-definition
  registration and guarantees cleanup before releasing the environment lock.
tags:
- deployment
- ecs
- cleanup
- locks
- verification
generated:
  at: '2026-07-24'
---

The provider-neutral deployment orchestrator tracks successful candidate task-definition registration and guarantees cleanup before releasing the environment lock. Cleanup uses a cancellation-independent two-minute timeout and joins cleanup errors with the original failure. AWS normal migration cleanup clears registration state; the deferred path covers hook and later-phase failures. Race tests, full make verify, and Floci tests passed.
