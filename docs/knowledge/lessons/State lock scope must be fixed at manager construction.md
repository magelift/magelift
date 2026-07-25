---
type: lesson
title: State lock scope must be fixed at manager construction
description: A deployment lock manager must bind project and environment into its object key and reject
  acquire calls for another scope.
tags:
- state
- locks
- safety
status: stable
generated:
  at: '2026-07-24'
---

A deployment lock manager must bind project and environment into its object key and reject acquire calls for another scope. Accepting a different scope can write mismatched metadata under the original lock key.
