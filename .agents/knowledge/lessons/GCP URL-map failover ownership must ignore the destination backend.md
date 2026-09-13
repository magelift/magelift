---
type: lesson
title: GCP URL-map failover ownership must ignore the destination backend
description: Pre-transition ownership checks that require ExpectedBackendServiceURL already match the failover target refuse a healthy primary URL map.
tags: [gcp, edge, failover, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-17
---

# GCP URL-map failover ownership must ignore the destination backend

Cell `20260817bk` applied a two-origin Cloud CDN front end, reached alias HTTPS
`200`, and proved the primary HTML title. Failover then failed with
`refusing to transition an unowned or drifted GCP URL map`.

`Start` copies the plan and sets `ExpectedBackendServiceURL` to the secondary
backend before `Transition`. The first `urlMapMatchesRequest` call used that
expected destination against a URL map whose default service was still the
primary. The map was owned and on an origin-group member; the predicate treated
the not-yet-failed-over state as drift.

Clear `ExpectedBackendServiceURL` for the pre-mutation ownership check. After
the URL-map update, require the destination backend. Do not reuse `bk`. The
corrected cell is `20260817bl`.

See [the live evidence](../../../docs/evidence/gcp-edge-failover-https-live-20260817bl.md).
