---
type: lesson
title: MageLift expired preview teardown validation 2026-07-18
description: Preview expiration is required for deployment safety, but a destroyed stack may already be
  expired.
tags:
- validation
- ttl
- teardown
status: stable
generated:
  at: '2026-07-24'
---

Preview expiration is required for deployment safety, but a destroyed stack may already be expired. A first attempt cleared the timestamp and failed because zero also violated the required-expiration rule. The correct boundary is a dedicated ValidateAllowExpiredPreview method used only for destroy planning; all normal planning still rejects missing or past preview TTLs.
