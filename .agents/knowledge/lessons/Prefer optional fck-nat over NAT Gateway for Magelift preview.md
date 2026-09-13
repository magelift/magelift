---
type: lesson
title: Prefer optional fck-nat over NAT Gateway for Magelift preview
description: Future Magelift AWS networking should offer fck-nat as natMode alternative to managed NAT
  Gateway for preview/low-cost.
tags:
- aws
- nat
- fck-nat
- cost
- preview
status: draft
decision_status: planned
generated:
  at: '2026-07-24'
---

Future Magelift AWS networking should offer fck-nat as natMode alternative to managed NAT Gateway for preview/low-cost. Explicit catalog choice, not silent swap. HA/production stay on NAT Gateway unless proven otherwise. NAT Gateway burned credits during failed acceptance apply.

# Related

* Relates to: Projects/magelift/Findings/AWS acceptance status 2026-07-19
