---
type: lesson
title: MageLift CLI lifecycle uses explicit AWS plans and locks
description: The CLI now resolves a named environment, converts explicit target.aws inputs and benchmark
  catalog values into a validated AWS stack Spec, and drives Pulumi Automation API preview/deploy/destroy/o...
tags:
- cli
- pulumi
- state-lock
- floci
- deployment
generated:
  at: '2026-07-24'
---

The CLI now resolves a named environment, converts explicit target.aws inputs and benchmark catalog values into a validated AWS stack Spec, and drives Pulumi Automation API preview/deploy/destroy/outputs through a narrow backend interface. Deploy and destroy acquire the deterministic S3 lock created by bootstrap; preview remains read-only. Production changes require --yes. MAGELIFT_AWS_ENDPOINT_URL routes the state S3 client to an AWS-compatible emulator such as Floci for account-free lock tests. Pulumi resource graphs still need mocks and real AWS matrices because Floci does not emulate the managed service graph.
