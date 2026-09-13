---
type: lesson
title: Cloud Armor GA import drops requestBodiesToExclude
description: Magento-safe Armor needs request-body exclusions; Compute v1 import stores GraphQL parsing and 64 KB inspection but not requestBodiesToExclude (beta REST only).
tags: [gcp, cloud-armor, waf, magento]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
---

# Cloud Armor GA import drops requestBodiesToExclude

`gcloud compute security-policies import` against Compute v1 stored Magento-safe
`STANDARD_WITH_GRAPHQL` JSON parsing, `64KB` body inspection, and the
`scannerdetection` deny rule. It did not persist `requestBodiesToExclude`
(`sqliBodyExcl=0` on the 2026-08-13 `20260813af` export).

Pulumi documents `requestBodies` as **Optional, Beta**. The v1 Go client
(`google.golang.org/api v0.291.0`) has no `RequestBodiesToExclude` field.
URI, query, and cookie exclusions are GA.

Do not claim Magento-safe Cloud Armor from a GA import until body exclusions
are written through the beta `patchRule` / `requestBodiesToExclude` path.

Verify with Compute beta GET. GA `security-policies export` still reports
`sqliBodyExcl=0` after a successful beta patch.

The 2026-08-13 `20260813ag` cell used that path: GA import plus beta
`patchRule` on six WAF rules, beta GET verify, backend attach, destroy, and
empty inventories (`waf=attached`).
