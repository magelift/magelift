---
type: lesson
title: Rollback copies promote signatures and Magento schema epoch
description: Deploy journal rows are often unsigned and epoch-less; rollback must look up Cosign metadata and schemaEpoch from the newest promote of the same digest, and production must observe Magento patch_list plus setup_module or fail closed.
tags: [rollback, release-journal, cosign, magento, schema]
status: stable
generated:
  by: cursor-grok/darwin
  at: 2026-08-19
---

# Rollback copies promote signatures and Magento schema epoch

`magelift deploy` appends `ActionDeploy` rows that historically omitted
`SignatureIdentity`, `SignatureIssuer`, and `schemaEpoch`. Rollback used to
require those fields on the selected sequence, so a KEEP stack that never ran
`magelift promote` failed with "no verified signature metadata" before
`RefuseIncompatibleRollback` ran.

Rollback now copies Cosign identity/issuer and schema epoch from the newest
journal row of the same digest that recorded them. Production `liveSchemaEpoch`
queries Magento `patch_list` plus `setup_module` through an **explicit**
dumpimport transport (`MAGELIFT_DUMPIMPORT_RUNNER=kube` with namespace and
pod/selector, or `MAGELIFT_DUMPIMPORT_HOST`). It does not dial localhost MySQL. Observation failure still records epoch 0 and rollback fails closed
(`ErrSchemaUnavailable`). Schema *mismatch* still needs two different recorded
epochs (two Magento schemas). Same-image rollback is compatible, not a mismatch
proof.

GCP `up` used to skip `magelift promote`, so KEEP journals stayed unsigned even
after rollback learned to copy promote metadata. Create-once now promotes the
pullable digest with `MAGELIFT_GCP_CERTIFICATE_IDENTITY` before
`deploy:candidate`. Promote still fail-closes on unsigned or mutable
references. Schema mismatch still needs two Magento schemas.

Do not reuse prefixes `gcha23`, `gcap24`–`gcap28`, `gcha28`–`gcha36`. Live
mismatch proof: [`Schema-mismatch rollback needs two Cosign-signed Magento epochs`](Schema-mismatch%20rollback%20needs%20two%20Cosign-signed%20Magento%20epochs.md).
