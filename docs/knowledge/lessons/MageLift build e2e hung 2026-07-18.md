---
type: lesson
title: MageLift build e2e hung 2026-07-18
description: The account-free scripts/build-e2e.sh built the PHP builder image successfully but the subsequent
  go run cmd/magelift build produced no output for more than 50 seconds and was interrupted.
tags:
- verification
- build
status: stable
generated:
  at: '2026-07-24'
---

The account-free scripts/build-e2e.sh built the PHP builder image successfully but the subsequent go run cmd/magelift build produced no output for more than 50 seconds and was interrupted. The script redirects CLI JSON to a temporary file, so the local build path needs a bounded timeout or progress diagnostics before this can be called a passing end-to-end check.
