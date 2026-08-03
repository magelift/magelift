---
type: lesson
title: MageLift release supply chain contract 2026-07-18
description: Tagged releases publish the PHP 8.2-8.5 runtime, builder, and FrankenPHP classic image matrix
  to GHCR with SBOM and SLSA provenance, keyless Cosign digest signatures, explicit digest verification,
  ...
tags:
- release
- cosign
- ghcr
- provenance
- security
generated:
  at: '2026-07-24'
---

Tagged releases publish the PHP 8.2-8.5 runtime, builder, and FrankenPHP classic image matrix to GHCR with SBOM and SLSA provenance, keyless Cosign digest signatures, explicit digest verification, and GitHub registry attestations. GoReleaser signs checksums with a Cosign Sigstore bundle; the self-updater requires checksums.txt.sigstore.json, verifies it against the MageLift release workflow identity for the exact tag, then verifies the archive SHA-256. Current Cosign installer v4.1.2 supplies Cosign 3.0.6 and actions/attest v4.2.0 is pinned.
