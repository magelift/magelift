---
type: lesson
title: MageLift Cosign CLI contract 2026-07-18
description: Official GitHub release v3.1.2, tagged and published on 2026-07-17, is the latest stable
  Cosign release as checked on 2026-07-18.
tags:
- cosign
- sigstore
- signing
- versions
status: stable
generated:
  at: '2026-07-24'
---

Official GitHub release v3.1.2, tagged and published on 2026-07-17, is the latest stable Cosign release as checked on 2026-07-18. The tag-specific v3.1.2 command reference confirms keyless digest signing remains cosign sign --yes REGISTRY/REPOSITORY@sha256:DIGEST. Identity-bound verification remains cosign verify --certificate-identity IDENTITY --certificate-oidc-issuer HTTPS_ISSUER REGISTRY/REPOSITORY@sha256:DIGEST. Always require a registry-qualified digest, never a tag, and suppress subprocess output at the orchestration boundary so registry or identity-provider errors cannot leak credentials. The local environment has no cosign executable on PATH, so syntax was verified from the official v3.1.2 tag documentation.
