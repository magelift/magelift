---
type: lesson
title: MageLift Floci OIDC identity test failure 2026-07-18
description: A proposed Floci integration test for bootstrap GitHub OIDC identity failed because Floci
  1.5.33 returns UnsupportedOperation for IAM GetOpenIDConnectProvider.
tags:
- floci
- iam
- oidc
- failure
generated:
  at: '2026-07-24'
---

A proposed Floci integration test for bootstrap GitHub OIDC identity failed because Floci 1.5.33 returns UnsupportedOperation for IAM GetOpenIDConnectProvider. The test was removed rather than weakening the production identity contract or silently skipping coverage. docs/bootstrap.md records that OIDC identity and Pulumi resource graphs still require the scheduled real-AWS matrix.
