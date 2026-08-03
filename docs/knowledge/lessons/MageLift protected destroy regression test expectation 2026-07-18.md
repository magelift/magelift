---
type: lesson
title: MageLift protected destroy regression test expectation 2026-07-18
description: After tightening the protected-destroy guard, the existing test still expected the old --yes
  wording and failed before behavior verification.
tags:
- testing
- regression
- safety
status: stable
generated:
  at: '2026-07-24'
---

After tightening the protected-destroy guard, the existing test still expected the old --yes wording and failed before behavior verification. Update safety regression assertions whenever a policy error is intentionally strengthened, then rerun focused CLI and stack tests.
