---
type: lesson
title: MageLift account-free secret and state safety paths 2026-07-18
description: 'The CLI secret commands use AWS Secrets Manager only: set reads at most 64 KiB from explicitly
  enabled stdin, never prints the value, list returns metadata, and remove requires --yes and uses the
  n...'
tags:
- magelift
- state
- secrets
- floci
- safety
generated:
  at: '2026-07-24'
---

The CLI secret commands use AWS Secrets Manager only: set reads at most 64 KiB from explicitly enabled stdin, never prints the value, list returns metadata, and remove requires --yes and uses the normal 30-day recovery window. State backup and restore copy durable S3 state objects under a timestamped backups prefix while holding the deployment lock; lock objects and prior backups are excluded, restore validates the complete marker and copies snapshot objects before deleting stale durable objects, and unlock requires --yes. Floci integration exercises bootstrap, S3 locks, and archive restore without an AWS account.
