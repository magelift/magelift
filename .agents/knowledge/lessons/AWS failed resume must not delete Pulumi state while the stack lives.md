---
type: lesson
title: AWS failed resume must not delete Pulumi state while the stack lives
description: Harness EXIT deleted the awf18 S3 state bucket after resume fingerprint mismatch, leaving a billed Magento stack with no Pulumi state.
tags:
- aws
- destroy
- acceptance
- pulumi
generated:
  by: cursor-grok
  at: '2026-08-18'
---

`scripts/aws-acceptance-local.sh` deletes an owned Pulumi state bucket on EXIT whenever `state_bucket_owned=1`, including failed `RESUME` before `created=1`. On `awf18`, resume refused a fingerprint mismatch, then the trap removed `s3://magelift-…-awf18-preview-state` while ECS/RDS/NAT/ALB/CloudFront were still running. Direct `magelift destroy` was then impossible. Cost-bearing cleanup had to go through owning-service deletes (`delete-db-instance --skip-final-snapshot`, ECS `--force`, CloudFront disable-then-delete). Do not invoke the harness with `KEEP=false` unless destroy will run. Prefer `magelift destroy --yes` against the live workdir while the backend still exists. Relates to light AWS smoke: destroy immediately after catalog PASS.
