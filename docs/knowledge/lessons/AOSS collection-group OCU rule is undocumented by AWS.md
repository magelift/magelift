---
type: lesson
title: AOSS collection-group OCU rule is undocumented by AWS
description: MageLift's serverless OCU step rule is empirically observed, not published by AWS — do not relax the validator against the docs.
tags:
- aws
- opensearch
- aoss
- validation
- honesty
status: stable
generated:
  by: cursor/darwin
  at: 2026-07-28
sources:
  - id: aws-serverless-scaling
    resource: https://docs.aws.amazon.com/opensearch-service/latest/developerguide/serverless-scaling.html
    title: Managing capacity limits for Amazon OpenSearch Serverless
  - id: aws-account-settings
    resource: https://docs.aws.amazon.com/opensearch-service/latest/APIReference/API_serverless_UpdateAccountSettings.html
    title: UpdateAccountSettings (account-level capacity API)
---

MageLift's `validServerlessOCU` in [`internal/cloud/aws/search/search.go`](/../internal/cloud/aws/search/search.go) enforces that collection-group capacity values are 1, 2, 4, 8, 16, or a multiple of 16 at or above 32. That step rule is **not published by AWS**.

The collection-group capacity reference[^aws-serverless-scaling] documents only a minimum floor of zero and a maximum floor of one for these fields — no step, no ceiling. The multiples-of-two granularity a reader will find belongs to the account-level settings API[^aws-account-settings], not to collection groups. MageLift also rejects a zero minimum, which AWS documents as valid for the field floor.

The rule came from a live collection-group rejection observed on 2026-07-19 under commit `b8b957e` (see also [AOSS collection-group OCU values cannot be zero](/lessons/AOSS%20collection-group%20OCU%20values%20cannot%20be%20zero.md)). Do not "fix" the validator by relaxing it to match the published model — a relaxed validator produces a create-time failure on a paid account.

This cell is unverifiable offline. When Phase 3 builds evidence tiering under TRUST-04, `docs/capability-matrix.md` should carry it as an unverifiable-offline row so the finding is not lost between phases. Re-verify on the next paid AWS pass before treating the step as AWS behaviour; the documented account-level ceiling of 1700 OCU is not enforced until that pass confirms it for collection groups.

# Related

- Validator and tests: `internal/cloud/aws/search/search.go`, `TestValidServerlessOCU`
- Prior observation: [AOSS collection-group OCU values cannot be zero](/lessons/AOSS%20collection-group%20OCU%20values%20cannot%20be%20zero.md)
