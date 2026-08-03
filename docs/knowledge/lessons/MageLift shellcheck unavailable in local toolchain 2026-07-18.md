---
type: lesson
title: MageLift shellcheck unavailable in local toolchain 2026-07-18
description: The local verification environment does not include shellcheck, so scripts/floci-test.sh
  and scripts/build-e2e.sh were not statically linted with shellcheck.
tags:
- verification
- shell
- ci
generated:
  at: '2026-07-24'
---

The local verification environment does not include shellcheck, so scripts/floci-test.sh and scripts/build-e2e.sh were not statically linted with shellcheck. Keep shellcheck in CI or install it before claiming shell-script lint coverage.
