---
type: lesson
title: MageLift protected destroy must not be bypassed 2026-07-18
description: Initial lifecycle logic treated --yes as sufficient to destroy a protected environment, which
  contradicted the documented protection contract.
tags:
- security
- lifecycle
- destruction
status: stable
generated:
  at: '2026-07-24'
---

Initial lifecycle logic treated --yes as sufficient to destroy a protected environment, which contradicted the documented protection contract. Destroy now rejects protected stacks until protection is explicitly disabled; the guard runs before backend access and has a regression test.
