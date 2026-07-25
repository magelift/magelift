---
type: lesson
title: MageLift ECS tag field SDK mismatch 2026-07-18
description: 'A compile failure occurred while testing ECS candidate fidelity: task-definition tags are
  returned on ecs.DescribeTaskDefinitionOutput.Tags, not types.TaskDefinition.Tags.'
tags:
- aws
- ecs
- sdk
- testing
generated:
  at: '2026-07-24'
---

A compile failure occurred while testing ECS candidate fidelity: task-definition tags are returned on ecs.DescribeTaskDefinitionOutput.Tags, not types.TaskDefinition.Tags. The implementation was corrected to request TaskDefinitionFieldTags and pass described.Tags into RegisterTaskDefinition.
