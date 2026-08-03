---
type: lesson
title: MageLift Phase 2 release build status 2026-07-18
description: MageLift now supports local prepared-context builds and registry push builds.
tags:
- phase-2
- buildkit
- cosign
- provenance
- sbom
status: draft
decision_status: in-progress
generated:
  at: '2026-07-24'
---

MageLift now supports local prepared-context builds and registry push builds. Push mode requires registry-pinned builder/runtime digests, a registry-qualified target, one or more platforms, and a separate HTTPS provenance URL with the full inspected checksum. BuildKit publishes SBOM and max SLSA provenance. OCI labels record source, revision, and MageLift version. The CLI signs the resulting digest with Cosign keyless signing. Cosign v3.1.2 is the verified current stable version. Query credentials are rejected from provenance URLs. Full make verify and Docker fixture E2E pass.
