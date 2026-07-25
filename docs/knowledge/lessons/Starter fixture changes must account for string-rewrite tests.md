---
type: lesson
title: Starter fixture changes must account for string-rewrite tests
description: Adding a branches field to MageLift starterConfig caused TestEnvironmentSelectionPrecedence
  to insert a duplicate YAML key because the test builds its fixture with strings.Replace.
tags:
- tests
- fixtures
- yaml
status: stable
generated:
  at: '2026-07-24'
---

Adding a branches field to MageLift starterConfig caused TestEnvironmentSelectionPrecedence to insert a duplicate YAML key because the test builds its fixture with strings.Replace. Keep the minimal starter neutral, demonstrate branch mapping in examples, and prefer structured fixtures when starter evolution would make textual rewrites fragile.
