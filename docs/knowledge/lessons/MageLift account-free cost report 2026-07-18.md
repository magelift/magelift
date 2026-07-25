---
type: lesson
title: MageLift account-free cost report 2026-07-18
description: The cost CLI intentionally reports capacity inputs as estimated and keeps monthlyTotalCents
  null.
tags:
- cli
- cost
- account-free
generated:
  at: '2026-07-24'
---

The cost CLI intentionally reports capacity inputs as estimated and keeps monthlyTotalCents null. It never calls AWS or treats monthlyBudgetCents as a forecast, because live regional prices and workload-dependent usage are not available without a pricing and measurement model.
