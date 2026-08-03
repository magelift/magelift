---
type: lesson
title: MageLift Phase 2 build boundary
description: Phase 2 build work must resolve immutable artifact inputs from project-level configuration,
  not from a selected environment.
tags:
- phase-2
- build
- artifact
- supply-chain
- configuration
generated:
  at: '2026-07-24'
---

Phase 2 build work must resolve immutable artifact inputs from project-level configuration, not from a selected environment. Config validation rejects environment overrides of project, application, or build. The PHP lifecycle executor runs the validated DAG topologically, uses argv-only process requests, respects typed retries, and never retries cancellation. Artifact manifests are canonical JSON written atomically. Registry matrix builds request SPDX SBOM plus SLSA v1 max provenance; signing remains release-only and digest-bound. Do not expose the public magelift build command until the versioned PHP runner protocol, BuildKit digest handoff, secret redaction, and cancellation contracts exist.
