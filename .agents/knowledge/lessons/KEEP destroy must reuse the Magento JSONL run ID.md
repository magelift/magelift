---
type: lesson
title: KEEP destroy must reuse the Magento JSONL run ID
description: ACCEPTANCE_EVIDENCE_RUN_ID is date-pid at source time. KEEP resume spawned a new run, so cleanup PASS could not VerifyRun with the Magento cells. Persist runId on the checkpoint and bind it before cleanup.
tags: [acceptance, jsonl, checkpoint, gcp, keep]
status: stable
generated:
  by: cursor-grok/darwin
  at: 2026-08-20
---

# KEEP destroy must reuse the Magento JSONL run ID

`VerifyRun` requires PASS cells and cleanup PASS on the **same** `runId`.
`lib-evidence.sh` defaults `ACCEPTANCE_EVIDENCE_RUN_ID` to `run-<utc>-<pid>`
when the script is sourced. KEEP `up` then later `RESUME=1` KEEP-unset destroy
is a second process, so EXIT wrote cleanup PASS on a different run than the
Magento cells.

`gcap28` Magento cells were `run-20260820t100542z-15777`. Destroy EXIT first
wrote cleanup PASS on `run-20260820t104129z-19940`. The Magento run got a
follow-up cleanup PASS only after independent inventory was empty.

Fix: `acceptance_checkpoint_bind_run_id` stores `runId` on the checkpoint and
reuses it when `MAGELIFT_ACCEPTANCE_RUN_ID` is unset. Prefix-scoped checkpoint
directories keep runs apart. Fingerprint mismatch still resets the file.

Do not seal SKIP KEEP-cleanup. Do not rewrite historical SKIP rows.
