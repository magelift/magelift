---
type: lesson
title: MageLift build e2e verified with host UID and GOTMPDIR 2026-07-18
description: The account-free build-e2e target passed after rebuilding the builder image, allowing the
  container runner to execute Git with safe.directory and running Go compilation with GOTMPDIR=/home/alex/go/...
tags:
- verification
- build
- docker
status: stable
generated:
  at: '2026-07-24'
---

The account-free build-e2e target passed after rebuilding the builder image, allowing the container runner to execute Git with safe.directory and running Go compilation with GOTMPDIR=/home/alex/go/tmp. The pipeline prepared the fixture, built the immutable local image, finalized the external manifest, and validated runtime UID and secret-file exclusion. The initial no-space failure came from the /tmp tmpfs, not the code.
