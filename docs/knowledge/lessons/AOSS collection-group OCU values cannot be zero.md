---
type: lesson
title: AOSS collection-group OCU values cannot be zero
description: CreateCollectionGroup rejects minIndexingCapacityInOCU=0.
tags:
- aws
- opensearch
- aoss
- acceptance
generated:
  at: '2026-07-24'
---

CreateCollectionGroup rejects minIndexingCapacityInOCU=0. Allowed: 1, 2, 4, 8, 16, or multiples of 16. Magelift preview defaults mins to 1 (fixed in b8b957e). Maxima must also be in that set (e.g. 6 is invalid).
