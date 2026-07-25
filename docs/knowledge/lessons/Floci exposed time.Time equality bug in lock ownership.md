---
type: lesson
title: Floci exposed time.Time equality bug in lock ownership
description: State lock Release compared decoded and in-memory time.Time values with ==.
tags:
- floci
- go
- time
- state-lock
- failure
generated:
  at: '2026-07-24'
---

State lock Release compared decoded and in-memory time.Time values with ==. AWS S3 JSON round trips can preserve the instant while changing location metadata, so ownership checks must use time.Time.Equal for semantic equality.
