---
status: specified
slug: preview-env-lifecycle
intent: intent.md
---

# Spec: long-running envs plus cheap short-lived previews

Auto-approved per the standing `/goal` instruction. Defaults taken:

- V1 preview promise per certified origin: database-backed queues
  and single-AZ failure domain on both; search disabled on AWS
  (already the matrix promise, code was behind), 1-replica
  OpenSearch workload on GCP (already the matrix shape, harness
  relies on it). Pinned in docs plus config tests, not just
  convention.
- Live proof is the CLI loop on the GCP session (create, deploy,
  verify, sweep with `--before`, protection refusal). Generated CI
  stays shape-tested; no real PRs are opened against the public
  repo in-session. One manual rehearsal plus shape tests answers
  the second open question: sufficient for v1.

## Requirements

### Requirement: preview defaults are a pinned promise

The preview preset SHALL resolve to database-backed queues and a
single-AZ failure domain on both certified origins, disabled search
on AWS, and the 1-replica OpenSearch workload on GCP; and
`docs/operations.md` SHALL state the promise. Config tests pin it.

#### Scenario: preview resolves cheap

- **WHEN** a preview environment resolves on either origin
- **THEN** queue is db-backed, the failure domain is single-AZ, AWS
  search is disabled, GCP runs the documented 1-replica workload,
  and the docs say so

### Requirement: live CLI loop on the GCP session

Inside the GCP packed session, the CLI SHALL drive one full loop:
`env create` with TTL plus budget, `deploy`, verify, `sweep
--before` destroying the stack, `env list` plus `status` truthful
throughout, and a protection refusal for a protected destroy.

#### Scenario: loop closes live

- **WHEN** the loop runs
- **THEN** the preview stack exists after deploy, is gone after
  sweep, listings match reality, and the protected destroy refuses

### Requirement: close cleanup stays guarded

Stale-event generation guards and production `--yes` gates SHALL
stay covered by shape tests; nothing in the live loop weakens them.

#### Scenario: guards hold

- **WHEN** the suite runs
- **THEN** stale-event and production-gate tests pass unchanged

## Design

Offline now: preview-default pinning (tests plus one docs
paragraph). Live in-session: sequential CLI loop after the harness
preview cells, on its own project name so state never collides
with the harness stack; `--before` avoids TTL waiting.

## Gotchas / policy flags

- The loop stack dies inside the session window; assert_clean must
  cover its project name too.
- Never open real PRs against the public repo for this proof.

## Open questions carried forward

None. Both intent questions are decided above.
