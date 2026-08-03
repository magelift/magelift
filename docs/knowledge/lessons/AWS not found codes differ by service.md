---
type: lesson
title: AWS not found codes differ by service
description: On 2026-07-18 the IAM bootstrap tests exposed that AWS not-found errors are service-specific.
tags:
- aws
- iam
- ssm
- errors
- tests
generated:
  at: '2026-07-24'
---

On 2026-07-18 the IAM bootstrap tests exposed that AWS not-found errors are service-specific. The shared classifier initially covered S3 and KMS codes but omitted IAM NoSuchEntity and SSM ParameterNotFound. The classifier now includes NotFound, NoSuchBucket, NoSuchEntity, ResourceNotFoundException, and ParameterNotFound variants.
