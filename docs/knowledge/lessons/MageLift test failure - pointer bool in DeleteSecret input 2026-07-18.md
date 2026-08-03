---
type: lesson
title: 'MageLift test failure: pointer bool in DeleteSecret input 2026-07-18'
description: A state secret-store unit test compared the AWS SDK v2 DeleteSecretInput ForceDeleteWithoutRecovery
  pointer directly to bool, causing a compile error.
tags:
- go
- aws-sdk-v2
- testing
- failed-attempt
generated:
  at: '2026-07-24'
---

A state secret-store unit test compared the AWS SDK v2 DeleteSecretInput ForceDeleteWithoutRecovery pointer directly to bool, causing a compile error. Use aws.ToBool on pointer fields in tests and production assertions. The test was corrected and targeted Go tests passed.
