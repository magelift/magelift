# GCP GKE Autopilot schema-mismatch leftover — 2026-08-20 (`gcap28`)

This is Magento-wired rollback-vs-schema on the packed Autopilot KEEP
`gcap28`. Same-image rollback is not this proof. Two Cosign-signed Magento
digests with different `schemaEpoch` values, then `magelift rollback
--ack-forward-only` refused `ErrSchemaMismatch`.

## Setup

- Digest A (already promoted/deployed on catalog):
  `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4`
- Create-once promote of A had no `schemaEpoch` (Magento was not reachable).
  Re-promote of A after Magento was live recorded `schemaEpoch: 171`
  (sequence 4).
- Live `setup_module` gained one row (`Magelift_SchemaMismatchProbe`).
  Observed epoch became 172 (`patch_list` + `setup_module`).
- Digest B: docker commit LABEL `magelift.schema-mismatch=gcap28` on A,
  push as `…/magento-249-rc1-schema-mismatch-gcap28@sha256:64fa223d…980c25`.
- Sign digest B with an impersonated Google SA Sigstore identity token
  (`--audiences=sigstore --include-email`; file stripped of CR/LF).
- Promote B recorded `schemaEpoch: 172` (sequence 5). Deploy B (sequence 6).

Dumpimport transport for epoch observation: kube mysql-client pod in
`default`, `MAGELIFT_DUMPIMPORT_RUNNER=kube` plus host/user/database/pod/
namespace/kubeconfig. Cosign verify of both digests used identity
`devops@…iam.gserviceaccount.com` / issuer `https://accounts.google.com`.

## Result

`magelift rollback --to-sequence 4 --ack-forward-only` exited 2:

```
rollback digest cannot run on the live schema: live schema epoch 172 is newer than digest schema epoch 171; rollback would cut over before Magento can run; restore the previous database or forward-fix the current schema
```

That is `releasejournal.ErrSchemaMismatch` / `RefuseIncompatibleRollback`.
No cutover of digest A ran.

## Non-claims

- Database migrations were not reversed (forward-only)
- Magento Quality Patches / Adobe patch-list change
- Public certified status for any runtime other than GKE Autopilot
