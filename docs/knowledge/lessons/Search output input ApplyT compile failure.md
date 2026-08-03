---
type: lesson
title: Search output input ApplyT compile failure
description: Pulumi StringInput does not expose ApplyT directly in the current SDK.
tags:
- pulumi
- go
- search
- failed-attempt
generated:
  at: '2026-07-24'
---

Pulumi StringInput does not expose ApplyT directly in the current SDK. Convert it to StringOutput with ToStringOutput before applying a policy transformation.
