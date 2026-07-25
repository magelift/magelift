---
type: lesson
title: MageLift Composer lint command unavailable 2026-07-18
description: The build package defines composer test and static analysis scripts but no composer lint
  script.
tags:
- php
- verification
status: stable
generated:
  at: '2026-07-24'
---

The build package defines composer test and static analysis scripts but no composer lint script. Running composer lint fails with Command lint is not defined; use composer validate, composer test, and the declared PHPStan/Psalm scripts instead of inventing a command.
