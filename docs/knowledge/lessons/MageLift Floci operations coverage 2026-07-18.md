---
type: lesson
title: MageLift Floci operations coverage 2026-07-18
description: Floci image 1.5.33 now exercises account-free Secrets Manager create, version update, read,
  list, and deletion flows; CloudWatch Logs group, stream, ingestion, and filtered tail behavior; and
  versi...
tags:
- floci
- aws
- testing
- restore
status: stable
generated:
  at: '2026-07-24'
---

Floci image 1.5.33 now exercises account-free Secrets Manager create, version update, read, list, and deletion flows; CloudWatch Logs group, stream, ingestion, and filtered tail behavior; and versioned S3 media writes, deletion markers, and restoration from an older object version. The existing Floci suite also covers S3/KMS bootstrap, state backup/restore, conditional locks, ECS runtime health, and ephemeral candidate registration. Documentation and the README describe the expanded coverage without treating it as real AWS certification.
