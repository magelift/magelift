---
type: lesson
title: GCP acceptance WIF ownership must survive resume
description: A retained GCP acceptance run must persist ownership of its newly created Workload Identity resources so resumed cleanup cannot leave them behind.
tags:
- gcp
- cleanup
- workload-identity
- resume
status: stable
generated:
  by: codex/desktop
  at: '2026-08-15'
---

The `gcha16` HA run created its WIF pool, provider, and service account after
confirming that the exact resources were absent. The first process retained the
stack intentionally. The resume process skipped bootstrap, so its in-memory
ownership flag returned to zero. Infrastructure cleanup completed and
`assert_clean` passed, but the WIF resources remained because that assertion did
not include IAM identity resources.

Acceptance ownership is now recorded in a marker inside the run workdir when
the exact WIF resources are absent before bootstrap. A resumed process restores
that marker and deletes the provider, pool, and service account in dependency
order. The marker is removed only after all three deletions succeed, so a
partial cleanup can resume safely. A pre-existing WIF identity still has no
marker and remains protected. See [GCP acceptance cleanup must own WIF and state bucket](GCP%20acceptance%20cleanup%20must%20own%20WIF%20and%20state%20bucket%202026-08-04.md).
