---
type: lesson
title: Cosign identity-token files must be a single-line JWT
description: A trailing newline in a Cosign --identity-token file makes Fulcio reject the Authorization header; strip CR/LF before signing.
tags: [cosign, sigstore, gcloud]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
---

# Cosign identity-token files must be a single-line JWT

`gcloud auth print-identity-token` writes a JWT plus a trailing newline.
Passing that file to `cosign sign --identity-token PATH` produced:

`Post "https://fulcio.sigstore.dev/api/v2/signingCert": net/http: invalid header field value for "Authorization"`

Strip CR/LF (`tr -d '\r\n'`) so the file is exactly the JWT. Audience remains
`sigstore`; Google impersonation still needs `--include-email`. Do not print
the token.

Documented impersonation shape (Sigstore Cosign overview, retrieved 2026-08-13):
`gcloud auth print-identity-token --audiences=sigstore --include-email --impersonate-service-account SA`.

# Related

* Relates to: [OCI republish does not copy Sigstore signatures](OCI%20republish%20does%20not%20copy%20Sigstore%20signatures.md)
* Relates to: [Cosign signing identity is Sigstore OIDC not the Magento cloud provider](Cosign%20signing%20identity%20is%20Sigstore%20OIDC%20not%20the%20Magento%20cloud%20provider.md)
