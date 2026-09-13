---
type: lesson
title: AOSS collection-group OCU values cannot be zero
description: CreateCollectionGroup rejects minIndexingCapacityInOCU=0.
tags:
- aws
- opensearch
- aoss
- acceptance
status: stable
generated:
  at: '2026-07-24'
---

CreateCollectionGroup rejects minIndexingCapacityInOCU=0. Allowed: 1, 2, 4, 8, 16, or multiples of 16. Magelift preview defaults mins to 1 (fixed in b8b957e). Maxima must also be in that set (e.g. 6 is invalid).

The allowed set is MageLift's empirically observed rule, not an AWS-documented step — see [AOSS collection-group OCU rule is undocumented by AWS](/lessons/AOSS%20collection-group%20OCU%20rule%20is%20undocumented%20by%20AWS.md).
