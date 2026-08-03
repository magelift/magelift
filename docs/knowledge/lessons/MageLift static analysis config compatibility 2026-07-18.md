---
type: lesson
title: MageLift static analysis config compatibility 2026-07-18
description: PHPStan 2.2.5 passes at level 8 with treatPhpDocTypesAsCertain=false because protocol/runtime
  validation intentionally narrows values after dynamic JSON.
tags:
- phpstan
- psalm
- php
- static-analysis
status: stable
generated:
  at: '2026-07-24'
---

PHPStan 2.2.5 passes at level 8 with treatPhpDocTypesAsCertain=false because protocol/runtime validation intentionally narrows values after dynamic JSON. Psalm 6.16.1 required issueHandlers errorLevel syntax; severity is ignored. Public library unused symbols and MissingOverrideAttribute are suppressed because the package supports PHP 8.2 where #[Override] is unavailable; no type-safety errors remain.
