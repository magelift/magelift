---
type: lesson
title: MageLift Floci ECS candidate contract 2026-07-18
description: make floci-test passed after adding ECS candidate registration coverage.
tags:
- floci
- aws
- ecs
- testing
generated:
  at: '2026-07-24'
---

make floci-test passed after adding ECS candidate registration coverage. Floci exercises DescribeTaskDefinition, RegisterTaskDefinition, and multi-container immutable image replacement without an AWS account. It remains an emulator and does not certify managed AWS services or GitHub OIDC.
