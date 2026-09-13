---
type: lesson
title: OCI republish does not copy Sigstore signatures
description: Pushing the same image digest to a new ECR or Artifact Registry repository does not copy Cosign/Rekor signatures; magelift promote then fails with no signatures found.
tags: [cosign, sigstore, ecr, acceptance, magento]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
---

# OCI republish does not copy Sigstore signatures

`cosign copy` or a fresh keyless `cosign sign` is required after a digest is
pushed to a new repository. Layer/manifest identity is not enough.

The Magento 2.4.9 RC1 digest `sha256:e82dcf767cf57492b1c5619eac708d9af8b2e66f7b144ff49e6b2513753db495`
was recovered and pushed to `magelift-acceptance-rc1-20260813`. Direct
`cosign verify` against that repository returned `no signatures found`. AWS
cell `20260813ah` therefore failed at `magelift promote` (`signed release
verification failed`, exit 3) after a create-only preview. No VPC/RDS/ECS was
created (`created=0`).

Do not treat a pullable digest as a signed release. Verify the **target
repository** reference with the identity and issuer the harness will pass to
promote.

# Related

* Relates to: [Cosign identity-token files must be a single-line JWT](Cosign%20identity-token%20files%20must%20be%20a%20single-line%20JWT.md)
