---
type: lesson
title: Inspect generated patch literals before correcting escaping
description: On 2026-07-18, an apply_patch intended to remove JSON escape characters failed because the
  file already contained correct raw JSON.
tags:
- workflow
- apply-patch
- escaping
generated:
  at: '2026-07-24'
---

On 2026-07-18, an apply_patch intended to remove JSON escape characters failed because the file already contained correct raw JSON. Tool input escaping had made the proposed patch look different from the written file. Re-read the exact lines before correcting string literals generated through apply_patch.
