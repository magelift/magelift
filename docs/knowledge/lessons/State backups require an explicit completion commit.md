---
type: lesson
title: State backups require an explicit completion commit
description: MageLift state backup prefixes can remain partially populated when an S3 copy fails.
tags:
- aws
- s3
- state
- recovery
status: stable
generated:
  at: '2026-07-24'
---

MageLift state backup prefixes can remain partially populated when an S3 copy fails. Restore must reject any prefix without the final .magelift-complete marker before deleting or copying current state. Backup writes the KMS-encrypted marker only after all durable objects copy successfully.
