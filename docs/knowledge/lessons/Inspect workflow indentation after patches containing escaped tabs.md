---
type: lesson
title: Inspect workflow indentation after patches containing escaped tabs
description: An apply_patch payload inserted literal tab indentation into a GitHub Actions block scalar.
tags:
- github-actions
- yaml
- failed-attempt
generated:
  at: '2026-07-24'
---

An apply_patch payload inserted literal tab indentation into a GitHub Actions block scalar. YAML forbids tabs for indentation. Inspect changed workflow lines with sed -n l or a YAML parser after any patch that contains escaped tab characters, then replace them with spaces.
