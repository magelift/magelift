---
type: lesson
title: Do not assume Ruby is installed for workflow YAML checks
description: The MageLift development environment does not provide Ruby, so Ruby Psych cannot be used
  as an ad hoc GitHub Actions syntax check.
tags:
- tooling
- github-actions
- failed-attempt
generated:
  at: '2026-07-24'
---

The MageLift development environment does not provide Ruby, so Ruby Psych cannot be used as an ad hoc GitHub Actions syntax check. Prefer actionlint when available, or parse YAML through an existing repository dependency without adding a toolchain solely for validation.
