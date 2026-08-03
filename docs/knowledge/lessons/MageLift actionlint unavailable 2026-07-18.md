---
type: lesson
title: MageLift actionlint unavailable 2026-07-18
description: The July 2026 environment has no actionlint executable.
tags:
- ci
- verification
status: stable
generated:
  at: '2026-07-24'
---

The July 2026 environment has no actionlint executable. Workflow checks use pinned zizmor in GitHub Actions plus YAML parsing and repository-native verification; do not add an ad hoc runtime solely for local actionlint.
