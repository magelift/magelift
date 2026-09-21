---
type: lesson
title: Cosign signing identity is Sigstore OIDC not the Magento cloud provider
description: magelift sign and promote discover the current gcloud or CI login; Cosign flags are advanced. Cloud-native image signatures are not a substitute.
tags: [cosign, sigstore, oidc, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-19
---

# Cosign signing identity is Sigstore OIDC not the Magento cloud provider

The YAML-only path is `magelift sign` and `magelift promote --digest` with no
Cosign flags. MageLift discovers a Sigstore identity token from, in order:

1. `MAGELIFT_COSIGN_IDENTITY_TOKEN_*` or `--identity-token-file` (CI overrides)
2. `gcloud auth print-identity-token --audiences=sigstore --include-email`,
   impersonating `MAGELIFT_SIGNING_SERVICE_ACCOUNT`,
   `CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT`, or the Magento CI service
   account `ml-<project>-<env>-ci@<gcp-project>.iam.gserviceaccount.com`
3. Ambient Cosign OIDC (GitHub Actions, or a one-time browser confirmation)

`magelift promote` verifies with `cosign verify --certificate-identity
--certificate-oidc-issuer` using claims from that same token (`email` or `sub`,
plus `iss`). It does not consume Artifact Analysis, Binary Authorization, AWS
Signer, or ECR/Artifact Registry native signatures. Do not loosen verify to a
`.*` identity regex.

A Google user account cannot mint a Sigstore-audience token. Impersonate the
bootstrap CI service account instead of asking operators for Cosign knowledge.
A Google SA token can still sign an ECR digest.

Merchant UX is broader than Cosign. Keep `doctor` → `bootstrap` → `deploy` as
the path; see
[Magelift commands hide cloud-devops from merchants](Magelift%20commands%20hide%20cloud-devops%20from%20merchants.md).

Strip CR/LF before Cosign. Acceptance harnesses may still pass explicit
identity flags through `scripts/acceptance/lib-cosign.sh`.

Debian does not ship a `cosign` apt package. `apt-get install cosign` fails,
which is what `magelift build --push` prints on a clean Debian host. Install
the upstream `.deb` (or put the Cosign binary that `providers install` already
cached onto `PATH`). `build --push` looks up `cosign` on `PATH`; it does not
use that cache by itself.

# Related

* Relates to: [Cosign identity-token files must be a single-line JWT](Cosign%20identity-token%20files%20must%20be%20a%20single-line%20JWT.md)
* Relates to: [OCI republish does not copy Sigstore signatures](OCI%20republish%20does%20not%20copy%20Sigstore%20signatures.md)
