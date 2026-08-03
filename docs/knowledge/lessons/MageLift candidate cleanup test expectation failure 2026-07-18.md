---
type: lesson
title: MageLift candidate cleanup test expectation failure 2026-07-18
description: The first targeted CLI test run failed because the expected workflow order did not include
  the newly required candidate cleanup step.
tags:
- testing
- failure
- deployment
generated:
  at: '2026-07-24'
---

The first targeted CLI test run failed because the expected workflow order did not include the newly required candidate cleanup step. The fake expectation was corrected, then targeted race tests, full make verify, and Floci tests passed. Keep workflow-order assertions synchronized when deployment safety steps are added.
