---
type: lesson
title: MageLift memory write query syntax failure 2026-07-18
description: A first attempt to write three Memgraph notes in one query failed because this Memgraph endpoint
  expects one statement per run and rejected a second MERGE after a RETURN-less clause.
tags:
- memgraph
- failure
generated:
  at: '2026-07-24'
---

A first attempt to write three Memgraph notes in one query failed because this Memgraph endpoint expects one statement per run and rejected a second MERGE after a RETURN-less clause. The notes were retried as separate MERGE statements.
