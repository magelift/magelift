---
type: lesson
title: MageLift shellcheck info diagnostics 2026-07-18
description: Shellcheck v0.11.0 reports SC2016 informational findings for intentional single-quoted PHP
  snippets in scripts/build-e2e.sh.
tags:
- shellcheck
- ci
- failure
generated:
  at: '2026-07-24'
---

Shellcheck v0.11.0 reports SC2016 informational findings for intentional single-quoted PHP snippets in scripts/build-e2e.sh. CI runs shellcheck with --severity=warning so informational quoting notes do not fail the build while warning and error findings remain fatal.
