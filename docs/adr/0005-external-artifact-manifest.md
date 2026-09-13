# ADR 0005: Artifact manifest lives outside the image

- Status: Accepted
- Date: 2026-08-22

## Context

The artifact manifest must name the OCI image by digest. Embedding the finalized manifest in the image would change that digest. Rebuild loops follow.

## Decision

The finalized artifact manifest is external to the application image. MageLift writes it after BuildKit returns the digest.

1. `prepare` records deterministic pre-digest metadata (source revision, Magento edition, PHP, modules, checksums, static content matrix, required capabilities).
2. `finalize` accepts only that prepared output plus BuildKit's `containerimage.digest`. It does not re-inspect the tree.

Neither step records secret values. Promotion, sign, and rollback use the digest in the external manifest.

`magelift build --push`, `sign`, and `promote` use the operator's current cloud or CI login. Cosign keyless identity is Sigstore OIDC (Actions, Google SA), not Artifact Registry or ECR native signatures. The YAML-only path does not require Cosign flags.

Cloud-native image signatures are not a substitute for MageLift's signed release journal.

## Consequences

The same signed digest and manifest pair can move through staging and production. Republishing a digest to another registry does not copy Sigstore signatures; resign or copy signatures explicitly.

## Alternatives considered

- Manifest inside the image: rejected (digest cycle).
- Cloud-provider signing as the Magento release identity: rejected (wrong trust root).

## Provenance

Original. Cosign/Sigstore public docs.
