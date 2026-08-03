---
type: lesson
title: MageLift static content matrix propagation 2026-07-18
description: The PHP lifecycle runner now consumes the typed build.staticContent matrix from the Go protocol.
tags:
- build
- magento
- static-content
- verification
generated:
  at: '2026-07-24'
---

The PHP lifecycle runner now consumes the typed build.staticContent matrix from the Go protocol. An empty matrix keeps Magento default static deployment; configured locale and theme pairs produce one explicit setup:static-content:deploy command per pair. Invalid empty locale or theme values fail before execution. Composer static analysis, Psalm, PHPUnit, full make verify, and Floci tests passed after this change.
