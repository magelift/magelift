---
type: lesson
title: Canonical slice copies must preserve empty JSON lists
description: The first real build E2E failed because canonicalRequest used append on a nil destination.
tags:
- go
- json
- protocol
- testing
generated:
  at: '2026-07-24'
---

The first real build E2E failed because canonicalRequest used append on a nil destination. An intentionally empty Go slice became nil and encoded as JSON null, while the strict PHP protocol requires an empty list. Canonicalization must preserve non-nil empty list semantics for cross-language contracts, and golden tests must cover empty arrays.
