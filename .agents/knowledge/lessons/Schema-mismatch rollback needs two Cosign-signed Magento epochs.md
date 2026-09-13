---
type: lesson
title: Schema-mismatch rollback needs two Cosign-signed Magento epochs
description: Same-image rollback is already-current, not mismatch proof. Record digest A schemaEpoch after Magento is reachable, bump live patch_list or setup_module, promote a second Cosign-signed digest B, then rollback to A must hit ErrSchemaMismatch.
tags: [rollback, schema, cosign, magento, gcp, acceptance]
status: stable
generated:
  by: cursor-grok/darwin
  at: 2026-08-20
verified:
  - by: cursor-grok/darwin
    at: 2026-08-20
---

# Schema-mismatch rollback needs two Cosign-signed Magento epochs

`RefuseIncompatibleRollback` compares live Magento `patch_list` plus
`setup_module` against the target digest's recorded `schemaEpoch`. Create-once
`magelift promote` often runs before Magento exists, so digest A is signed but
epoch 0. Re-promote A after dumpimport can query Cloud SQL. Then change the
live epoch (one extra `setup_module` row is enough) and promote a **different**
Cosign-signed digest B.

`gcap28` (2026-08-20): A `schemaEpoch` 171, B 172, `rollback --to-sequence 4
--ack-forward-only` exited 2 with `cannot run on the live schema`. Signing B
used an impersonated Google SA identity token (`--audiences=sigstore
--include-email`) written as a single-line JWT.

Same-image rollback of A while A is current returns `already current` and is
not this proof. Do not reuse `gcap28`.
