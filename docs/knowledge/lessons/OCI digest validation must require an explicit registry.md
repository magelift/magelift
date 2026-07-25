---
type: lesson
title: OCI digest validation must require an explicit registry
description: A Cosign boundary test failed because a syntactically valid first path segment such as team
  was accepted as a registry.
tags:
- cosign
- oci
- validation
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

A Cosign boundary test failed because a syntactically valid first path segment such as team was accepted as a registry. Digest-only validation must require the first segment to be localhost or contain a dot or port separator, in addition to validating repository and sha256 grammar. This prevents Docker-style implicit registry resolution.
