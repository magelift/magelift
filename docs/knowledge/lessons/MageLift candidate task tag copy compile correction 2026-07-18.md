---
type: lesson
title: MageLift candidate task tag copy compile correction 2026-07-18
description: The first candidate task-definition preservation patch assumed TaskDefinition.Tags and pointer
  EphemeralStorage.SizeInGiB, but the AWS SDK exposes tags on DescribeTaskDefinitionOutput and SizeInGiB...
tags:
- magelift
- aws-sdk
- ecs
- candidate
- failure
generated:
  at: '2026-07-24'
---

The first candidate task-definition preservation patch assumed TaskDefinition.Tags and pointer EphemeralStorage.SizeInGiB, but the AWS SDK exposes tags on DescribeTaskDefinitionOutput and SizeInGiB as int32. Correct implementation requests TAGS in DescribeTaskDefinitionInput, copies described.Tags, and uses the SDK field types.
