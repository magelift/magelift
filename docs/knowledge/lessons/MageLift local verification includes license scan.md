---
type: lesson
title: MageLift local verification includes license scan
description: make verify now includes go-licenses/v2@v2.0.1 with forbidden and unknown license types rejected.
tags:
- verification
- license
- go
generated:
  at: '2026-07-24'
---

make verify now includes go-licenses/v2@v2.0.1 with forbidden and unknown license types rejected. The correct module path is github.com/google/go-licenses/v2; invoking the unversioned post-v2 path fails before analysis.
