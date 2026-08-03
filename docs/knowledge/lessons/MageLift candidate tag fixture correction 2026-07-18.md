---
type: lesson
title: MageLift candidate tag fixture correction 2026-07-18
description: An existing deployment candidate test fixture also modeled tags on types.TaskDefinition,
  but the AWS SDK returns tags separately on DescribeTaskDefinitionOutput.
tags:
- magelift
- aws-sdk
- ecs
- test
- failure
generated:
  at: '2026-07-24'
---

An existing deployment candidate test fixture also modeled tags on types.TaskDefinition, but the AWS SDK returns tags separately on DescribeTaskDefinitionOutput. Moving fixture tags to the deployment mock output restored the targeted operations test.
