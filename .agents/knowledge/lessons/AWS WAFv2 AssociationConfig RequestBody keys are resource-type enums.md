---
type: lesson
title: AWS WAFv2 AssociationConfig RequestBody keys are resource-type enums
description: create-web-acl AssociationConfig.RequestBody map keys must be CLOUDFRONT, not CloudFront; the live 20260813v Magento-safe WAF cell failed ValidationException before the WebACL existed.
tags: [aws, waf, cloudfront, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
sources:
  - id: aws-association-config
    resource: https://docs.aws.amazon.com/waf/latest/APIReference/API_AssociationConfig.html
    title: AWS WAFv2 AssociationConfig RequestBody keys
---

The Magento-safe WAF cell `20260813v` issued ACM, then `CreateWebACL` failed:

`Map keys must satisfy enum value set: [..., CLOUDFRONT, ...]`.

The CLI JSON used `RequestBody.CloudFront`. The API map key is the
`AssociatedResourceType` enum `CLOUDFRONT`. Pulumi's `Cloudfront` field is a
struct name and still maps correctly; the AWS CLI file payload does not.

Emit `"CLOUDFRONT": {"DefaultSizeInspectionLimit":"KB_64"}`. Do not reuse
`20260813v`.
