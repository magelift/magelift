---
type: lesson
title: MageLift Floci test invocation requires build tag 2026-07-18
description: Running go test ./tests/floci directly skips all files because Floci integration files require
  the floci build tag. Use make floci-test-aws.
tags:
- magelift
- floci
- testing
- failure
generated:
  at: '2026-07-24'
---

Running go test ./tests/floci directly skips all files because Floci integration files require the floci build tag. The supported account-free command is make floci-test-aws, which runs the pinned Floci environment and scripts/floci-test-aws.sh. make local-gates also runs that suite plus Pulumi mocks, the offline harness, and floci-gcp.
