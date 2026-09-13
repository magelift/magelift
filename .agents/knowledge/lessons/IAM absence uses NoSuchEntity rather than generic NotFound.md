---
type: lesson
title: IAM absence uses NoSuchEntity rather than generic NotFound
description: The first IAM/OIDC bootstrap tests failed because the shared AWS absence helper did not recognize
  IAM NoSuchEntity responses.
tags:
- aws
- iam
- idempotency
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

The first IAM/OIDC bootstrap tests failed because the shared AWS absence helper did not recognize IAM NoSuchEntity responses. IAM GetRole, GetPolicy, and GetOpenIDConnectProvider use NoSuchEntity. Keep service-specific absence classification explicit so only true absence triggers creation and access errors are never mistaken for missing resources.
