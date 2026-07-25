---
type: lesson
title: Capability requirement validation must allow absent manifests
description: Making an empty RequiredRuntimeCapabilities list invalid broke config-to-stack planning because
  that path does not yet ingest the artifact manifest.
tags:
- capabilities
- stack
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

Making an empty RequiredRuntimeCapabilities list invalid broke config-to-stack planning because that path does not yet ingest the artifact manifest. Capability validation should reject unsupported declared requirements but permit an empty list until manifest ingestion is mandatory at the deployment boundary.
