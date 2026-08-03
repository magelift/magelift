---
type: lesson
title: Pulumi generic ToOutput ApplyT uses interface input
description: pulumi.ToOutput on a custom slice can produce a generic output whose ApplyT callback must
  accept interface{}, not the concrete slice type.
tags:
- pulumi
- go
- runtime
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

pulumi.ToOutput on a custom slice can produce a generic output whose ApplyT callback must accept interface{}, not the concrete slice type. Normalize the callback input with a type assertion when one function handles both generic static output and pulumi.All output paths.
