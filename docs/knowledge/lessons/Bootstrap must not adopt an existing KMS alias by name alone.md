---
type: lesson
title: Bootstrap must not adopt an existing KMS alias by name alone
description: An expected MageLift KMS alias can already resolve to a manually managed or unrelated key.
tags:
- aws
- kms
- bootstrap
- ownership
status: stable
generated:
  at: '2026-07-24'
---

An expected MageLift KMS alias can already resolve to a manually managed or unrelated key. Before tagging, enabling rotation, or configuring the state bucket, bootstrap must require the key's magelift:bootstrap-id tag to match the selected account, region, project, and environment.
