---
type: lesson
title: CloudFront origin groups reject write methods on the default cache behavior
description: AWS CreateDistribution rejects origin-group default cache behaviors that allow POST/PUT/PATCH/DELETE; GET/HEAD/OPTIONS is the live contract.
tags: [aws, cloudfront, edge, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
sources:
  - id: cloudfront-live-20260813n
    resource: docs/evidence/aws-cloudfront-control-plane-live-2026-08-13.md
    title: AWS CloudFront control-plane live 2026-08-13
---

The first `20260813n` CloudFront cell failed at Apply because the default cache
behavior listed write methods. AWS requires origin-group behaviors to allow
only GET, HEAD, and OPTIONS. Single-origin distributions may still use a wider
method set. Keep that split in the translator; do not treat a fake-client
success as proof of the live API contract.
