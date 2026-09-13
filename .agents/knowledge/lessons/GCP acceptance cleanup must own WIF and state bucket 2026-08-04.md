---
type: lesson
title: GCP acceptance cleanup must own WIF and state bucket
description: GCP acceptance runs must preserve pre-existing Workload Identity resources while deleting only their own identity and Pulumi state bucket.
tags:
- gcp
- pulumi
- cleanup
- workload-identity
status: stable
generated:
  by: codex/desktop
  at: '2026-08-04'
---

An earlier acceptance retry reused a Workload Identity pool left by a failed
run. Its GitHub attribute condition belonged to a different repository, so
bootstrap failed with an attribute-condition mismatch. Manually deleting that
exact pool, provider, and service account unblocked the run, but broad IAM
cleanup would be unsafe in a user project.

The harness now checks the exact pool, provider, and service account before
bootstrap. It deletes them only when all three were absent at the start. It
also creates the exact Pulumi GCS backend bucket used by the stack bootstrap,
removes versioned objects on exit, and runs `assert_clean` after the producer
resource soak. A failed Pulumi destroy due to Cloud SQL Service Networking
producer lag is expected to enter force cleanup, not to skip the final scan.

For a retained run, the acceptance-owned WIF decision must survive the process
boundary. The harness persists a run-workdir marker and restores it on resume;
it removes the marker only after the provider, pool, and service account are
all deleted. The marker does not adopt an identity that existed before the
run. See [GCP acceptance WIF ownership must survive resume](GCP%20acceptance%20WIF%20ownership%20must%20survive%20resume%202026-08-15.md).
