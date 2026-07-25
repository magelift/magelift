---
type: lesson
title: MageLift IAM and SSM SDK versions 2026-07-18
description: The official Go module proxy checked on 2026-07-18 reports github.com/aws/aws-sdk-go-v2/service/iam
  v1.55.1, published 2026-07-13, and service/ssm v1.72.0, published 2026-07-14.
tags:
- aws
- iam
- ssm
- versions
- bootstrap
status: stable
generated:
  at: '2026-07-24'
---

The official Go module proxy checked on 2026-07-18 reports github.com/aws/aws-sdk-go-v2/service/iam v1.55.1, published 2026-07-13, and service/ssm v1.72.0, published 2026-07-14. IAM OIDC, role, and policy operations should remain behind narrow injected interfaces. SSM bootstrap metadata uses String parameters containing identifiers only, never credentials or secret values.
