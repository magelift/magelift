---
type: lesson
title: GitHub CI gofmt must enumerate Go files
description: gofmt -w dot is not a portable recursive formatting command for CI.
tags:
- ci
- go
- formatting
generated:
  at: '2026-07-24'
---

gofmt -w dot is not a portable recursive formatting command for CI. Enumerate tracked Go files with git ls-files, format them, then run git diff --exit-code so unformatted changes fail the job.
