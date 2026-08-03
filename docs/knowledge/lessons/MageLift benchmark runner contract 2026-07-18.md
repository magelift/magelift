---
type: lesson
title: MageLift benchmark runner contract 2026-07-18
description: Added internal/benchmark HTTP runner and `magelift benchmark run`.
tags:
- benchmark
- capacity
- testing
status: stable
generated:
  at: '2026-07-24'
---

Added internal/benchmark HTTP runner and `magelift benchmark run`. Reports catalog identity, request mix, concurrency, samples, success/failure classes, p50/p95/p99 latency, throughput, explicit unpriced cost inputs, and database/search/queue/cache engine metadata captured by CLI flags. It uses a deadline-bound context and no load-test dependency; preset defaults must not change until reports are paired with current regional pricing and representative Magento acceptance evidence.
