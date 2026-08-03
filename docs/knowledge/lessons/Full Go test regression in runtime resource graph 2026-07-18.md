---
type: lesson
title: Full Go test regression in runtime resource graph 2026-07-18
description: After adding pricing, upgrade, and release configuration, go test ./...
tags:
- testing
- runtime
- regression
generated:
  at: '2026-07-24'
---

After adding pricing, upgrade, and release configuration, go test ./... failed in an existing runtime test: TestRuntimeAttachesExistingSecurityGroupAndTargetGroup panicked while type-asserting a nil resource property. The failure is unrelated to the edited packages and must be diagnosed before completion.
