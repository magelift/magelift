---
type: lesson
title: MageLift CI dependency review gate 2026-07-18
description: Pull requests run actions/dependency-review-action v5.0.0 pinned to commit a1d282b36b6f3519aa1f3fc636f609c47dddb294.
tags:
- ci
- security
- dependabot
- floci
status: stable
generated:
  at: '2026-07-24'
---

Pull requests run actions/dependency-review-action v5.0.0 pinned to commit a1d282b36b6f3519aa1f3fc636f609c47dddb294. The job runs only for pull_request events, has contents:read permission, and fails for high or critical dependency risk. Floci 1.5.33 is the current upstream release as of 2026-07-18 and remains pinned by digest in compose and CI. make workflow-check, make verify, and make floci-test passed after the gate was added.
