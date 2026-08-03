---
type: lesson
title: MageLift target validation boundary 2026-07-18
description: internal/cloud/aws/stack.PlanFromConfig now validates the selected AWS provider/runtime and
  invokes internal/cloud/aws/target.Target.Validate with the preset topology before constructing an AWS
  sta...
tags:
- architecture
- provider-boundary
- pulumi
- validation
status: stable
generated:
  at: '2026-07-24'
---

internal/cloud/aws/stack.PlanFromConfig now validates the selected AWS provider/runtime and invokes internal/cloud/aws/target.Target.Validate with the preset topology before constructing an AWS stack spec. This preserves the future provider/runtime seam while preventing non-AWS plans and production-preview topology mismatches from reaching Pulumi. Regression tests cover both failures. Full make verify and make floci-test passed.
