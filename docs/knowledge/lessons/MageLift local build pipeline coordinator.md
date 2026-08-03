---
type: lesson
title: MageLift local build pipeline coordinator
description: 'internal/buildpipeline coordinates immutable local image builds: buildplan PrepareRequest
  uses /workspace, pinned isolated builder prepares into a private output, an embedded canonical application
  ...'
tags:
- build
- pipeline
- security
- artifact-manifest
status: stable
generated:
  at: '2026-07-24'
---

internal/buildpipeline coordinates immutable local image builds: buildplan PrepareRequest uses /workspace, pinned isolated builder prepares into a private output, an embedded canonical application Dockerfile builds that output with a pinned runtime through BuildKit load, then finalize reuses the same output with network disabled. It strictly decodes protocol responses, verifies image digest and manifest SHA/file containment, and atomically copies only the external manifest to an artifact directory outside the source tree. Builder/runtime pins accept bare sha256 local image IDs or repository@sha256 references. Composer auth is sent only to prepare.
