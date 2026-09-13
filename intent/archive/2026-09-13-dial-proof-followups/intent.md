---
status: accepted
slug: dial-proof-followups
parent: lean-core-provider-boundary (archived 2026-09-13)
---

# Intent: close the Dial proof test gaps

## Problem

The archived Dial proof report carries one WARNING and one SUGGESTION,
both tests: no test pins subprocess `Plan` equality with in-process
`Plan` for the same input (the `configFromMap` drift surface), and the
SDK version handshake refusal is helper-tested only, with no Dial-level
mismatch test. Order 6 of the roadmap demands contract tests and local
subprocess proof; these two close it.

## Evidence

`intent/archive/2026-09-13-lean-core-provider-boundary/report.md`,
Findings: plan-equality WARNING at `internal/providerhost/host_test.go`,
handshake-mismatch SUGGESTION at `internal/providerhost/session.go`.

## Proposed outcome

A subprocess `Plan` over real Dial equals the in-process `Plan` for
the same GCP fixture input, pinned by a test. A Dial against a plugin
binary reporting a wrong SDK API version fails closed, pinned by a
Dial-level test with a fake plugin binary. Resume guidance stays out:
the in-process path has no resume either, so subprocess-only guidance
would be inconsistent.

## Affected users and systems

`internal/providerhost` tests only. No behavior change.

## Constraints

Local only. No new RPCs, no behavior change, no live cloud.

## Out of scope

Live subprocess activation (Phase 2, GCP packed session). Resume
guidance on either path. Community plugin tests.

## Open questions

None.
