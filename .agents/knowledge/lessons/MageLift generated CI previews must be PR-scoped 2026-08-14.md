---
type: lesson
title: MageLift generated CI previews must be PR-scoped 2026-08-14
description: A single configured preview environment lets concurrent pull requests collide and lets one close event destroy another request's stack.
tags: [cli, ci, github-actions, preview, pulumi]
status: stable
generated:
  by: codex/codex
  at: 2026-08-14
---

The generated workflow currently reads one configured preview environment for
every pull request. The CLI can create unique names manually, but its built-in
workflow does not derive identity from the pull request number, so concurrent
requests share a stack and close cleanup has no request ownership to verify.

Use repository plus pull request number as the logical identity. Keep the
branch name as diagnostic metadata and the commit digest as the deployment
generation. Pulumi stack keys, ownership markers, domains, and expiration
records must derive from that identity. Serialize apply and close jobs per
pull request, and refuse stale cleanup before any provider mutation.

The follow-up OpenSpec change is
`pr-scoped-preview-environments`. It also records that CI generation is
currently AWS ECS-only even though the configuration and provider registry
support other first-party targets; GCP is the first new workflow path.
