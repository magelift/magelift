---
type: lesson
title: MageLift Pulumi module versions 2026-07-18
description: The official Go module proxy checked on 2026-07-18 reports github.com/pulumi/pulumi/sdk/v3
  v3.253.0, published 2026-07-14, and github.com/pulumi/pulumi-aws/sdk/v7 v7.37.0, published 2026-07-15.
tags:
- pulumi
- aws
- versions
- phase-3
status: stable
generated:
  at: '2026-07-24'
---

The official Go module proxy checked on 2026-07-18 reports github.com/pulumi/pulumi/sdk/v3 v3.253.0, published 2026-07-14, and github.com/pulumi/pulumi-aws/sdk/v7 v7.37.0, published 2026-07-15. Context7 official Pulumi sources confirm Automation API stack Preview and Up remain context-aware operations, and AWS provider Go imports use sdk/v7/go/aws. Do not add either module before code imports it. Keep provider schemas inside AWS target packages, not user configuration or provider-neutral SDK contracts.
