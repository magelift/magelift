---
type: lesson
title: Docker Bake generated target inheritance requires a local matrix
description: A Docker Bake target whose name references a matrix value must declare that matrix itself.
tags:
- docker
- buildx
- bake
- failed-attempt
generated:
  at: '2026-07-24'
---

A Docker Bake target whose name references a matrix value must declare that matrix itself. Inheriting a matrix target does not put its matrix variables in scope for the child name. Keep shared matrix records in one Bake variable and declare matrix expansion on each generated target.
