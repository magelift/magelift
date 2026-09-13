---
status: draft
slug: preview-env-lifecycle
---

# Intent: long-running envs plus cheap short-lived previews

## Problem

Teams need production and staging that stay up and are hard to destroy by accident, plus feature previews that spin up from a pull request and disappear when the branch closes. The pieces exist (`env create`, TTL, budgets, sweep, CI generate) but the lifecycle has not been proven as one loop an agency can trust: create from PR, deploy a digest, verify, destroy on close, never touch production.

## Evidence

`magelift env create` adds a validated overlay with account, preset, class, domain, TTL, budget, protection, branches. `env sweep` destroys only expired unprotected `class: preview` envs; production and protected are always skipped. Preview requires TTL plus budget before infra creation. `magelift ci generate` writes a deterministic workflow: labeled PR previews, close cleanup with generation guard, staging on main, hourly sweep, gated manual production. `openspec/specs/preview-environment-identity/spec.md` defines PR identity. A full PR open to PR close loop against a live provider with destroy plus `assert_clean`: exercised in CI shape tests, live end-to-end cadence not checked in this session.

## Proposed outcome

An agency runs the whole loop from the CLI and generated CI: PR preview up on a certified origin with TTL and budget, staging on main, production gated and protected, expired previews swept without touching long-running envs. `env list`, `status`, `sweep --dry-run` report truthfully without cloud calls where documented. Close cleanup refuses stale events before it can destroy a newer deployment. Protection plus `--yes` gates hold for production and protected destroys.

## Affected users and systems

Agencies running several shops, reviewers testing features. `magelift env`, generated GitHub workflows, deployment locks, release journal, `docs/operations.md`, user skill `magelift-operate`.

## Constraints

Destroy previews on close; no orphan stacks. Preview identity derives from repository plus PR number so renames and new commits reuse the stack. Stale-event guard stays. Long-lived state backup/restore remains available for staging/production; previews stay disposable. Live proof attaches to packed GCP sessions where possible to avoid extra origins.

## Out of scope

Non-GitHub CI systems, per-commit environments, production blue/green semantics, multi-region previews.

## Open questions

Which preview defaults (queue `db`, search disabled, single-AZ fck-nat or equivalent) are the v1 promise per certified origin? Do we need a live PR-close destroy proof for v1, or do shape tests plus one manual rehearsal suffice?
