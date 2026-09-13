---
type: lesson
title: Acceptance JSONL PASS needs reuse-boundary identities
description: Unsealed Magento catalog PASS becomes SKIP unless the harness exports fixture, backup, observability, edge, schema, migration, state backend, and checkpoint fingerprint; dry-run PASS must stay SKIP.
tags: [acceptance, evidence, jsonl, certification, gcp, aws]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-20
---

# Acceptance JSONL PASS needs reuse-boundary identities

`gcap25` JSONL could not close RC1 5.1 because `append_shared_row` downgraded
PASS to SKIP when `MAGELIFT_ACCEPTANCE_FIXTURE_ID`, `BACKUP_SET`,
`OBSERVABILITY_SETUP`, `EDGE_SETUP`, `SCHEMA_FINGERPRINT`,
`MIGRATION_FINGERPRINT`, `STATE_BACKEND`, or
`MAGELIFT_ACCEPTANCE_CHECKPOINT_FINGERPRINT` were empty. Markdown catalog PASS
is not a sealed bundle.

GCP and AWS harnesses must call `acceptance_export_reuse_boundary` after the
checkpoint fingerprint exists. Values are stack identities (seed sha256,
declared Cloud SQL/RDS automated backup policy, Pulumi backend URL), not
restore or traffic proofs. `MAGELIFT_ACCEPTANCE_DRY_RUN=1` must still write
SKIP even when those fields are present, so a placeholder digest cannot be
sealed as live Magento evidence.

KEEP cleanup remains SKIP until KEEP is unset and destroy/`assert_clean` run.
