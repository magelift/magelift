---
type: lesson
title: OVH OKMS live certification needs an existing service domain
description: OVH Secret Manager live evidence cannot be fabricated when the authenticated account has no OKMS service to exercise.
tags: [ovh, okms, secret-manager, acceptance, cost-control]
status: stable
generated:
  by: codex
  at: 2026-08-12
---

# Rule

Before an OVHcloud Secret Manager live cell, require an authenticated OKMS
service identity and region that the operator has already provisioned. If the
owning-service inventory is empty, record the typed prerequisite gap and stop
before mutation; do not create a paid OKMS service solely to manufacture a
certification row.

# Why

The authenticated OVH CLI returned no OKMS service domains during the
2026-08-12 audit. The OVH SDK adapter and offline lifecycle tests exist, but
there was no provider service identity against which to run the Secret Manager
cell. The implementation remains evidence-gated until an operator supplies a
real OKMS domain; Object Storage cleanup and inventory must still run for any
separate disposable test.
