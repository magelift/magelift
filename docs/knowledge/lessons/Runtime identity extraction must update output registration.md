---
type: lesson
title: Runtime identity extraction must update output registration
description: After extracting ECS IAM roles into runtime.Identity, the first compile failed because Runtime
  New still registered outputs from removed local executionRole, taskRole, and deploymentRole variables.
tags:
- go
- runtime
- refactor
- test-failure
status: stable
generated:
  at: '2026-07-24'
---

After extracting ECS IAM roles into runtime.Identity, the first compile failed because Runtime New still registered outputs from removed local executionRole, taskRole, and deploymentRole variables. Use the identity outputs consistently in task definitions and component output registration.
