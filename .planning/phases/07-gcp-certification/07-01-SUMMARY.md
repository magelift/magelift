---
phase: 07-gcp-certification
plan: 01
subsystem: gcp-bootstrap
tags: [gcp, wif, secret-manager, composer, github-actions, act]

requires: []
provides:
  - GCP WIF BuildIdentityPlan + Ensure with injectable fake client
  - Composer gcp-secret-manager:// resolution via AccessSecretVersion
  - Act-only WIF smoke workflow + documented gcloud exchange path
affects:
  - 07-gcp-certification live bootstrap cell
  - GCP-01 CI auth evidence
  - GCP-02 Composer build path

tech-stack:
  added: []
  patterns:
    - AWS-shaped injectable WIFAPI for unit-proven Ensure
    - secretref.Resolver.GCPSecretManager wired like AWS Secrets Manager

key-files:
  created:
    - internal/cloud/gcp/bootstrap/identity.go
    - internal/cloud/gcp/bootstrap/identity_test.go
    - internal/cloud/gcp/bootstrap/wif_client.go
    - internal/cloud/gcp/secrets/secrets_test.go
    - .github/workflows/gcp-wif-act-smoke.yml
  modified:
    - internal/cloud/gcp/bootstrap/bootstrap.go
    - internal/cloud/gcp/secrets/secrets.go
    - internal/cloud/gcp/ops/day2.go
    - internal/cli/build.go
    - internal/cli/build_test.go
    - internal/cli/root.go
    - docs/gcp-experimental.md
    - docs/gcp-acceptance.md

key-decisions:
  - "WIF attribute condition locked to assertion.repository == 'acourtiol/magelift' (T-07-01)"
  - "Bootstrap Details.wif is structured map {pool,provider,serviceAccount} never deferred"
  - "CI proof is Act-only workflow_dispatch until hosted Actions minutes return"
  - "No new Go modules — google.golang.org/api IAM + CRM + existing secretmanager"

patterns-established:
  - "Pattern: fake WIFAPI for offline Ensure idempotency + loud failure tests"
  - "Pattern: gcpComposerSecretAdapter bridges Store.GetSecretValue into composerSecretProvider"

requirements-completed: [GCP-01, GCP-02]

coverage:
  - id: D1
    description: WIF pool + GitHub OIDC provider + CI SA binding Ensure unit-proven
    requirement: GCP-01
    verification:
      - kind: unit
        ref: "go test ./internal/cloud/gcp/bootstrap/ -run 'WIF|Identity'"
        status: pass
    human_judgment: false
  - id: D2
    description: Composer gcp-secret-manager:// resolves via AccessSecretVersion with loud failures
    requirement: GCP-02
    verification:
      - kind: unit
        ref: "go test ./internal/cloud/gcp/secrets/ ./internal/cli/ -run 'Secret|Composer|GCP'"
        status: pass
    human_judgment: false
  - id: D3
    description: Act-only WIF smoke workflow without SA key material + docs exchange path
    requirement: GCP-01
    verification:
      - kind: other
        ref: "rg google-github-actions/auth@v3 .github/workflows/gcp-wif-act-smoke.yml"
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-30
status: complete
---

# Phase 7 Plan 01: WIF + Composer SM Offline Foundations Summary

**Offline WIF Ensure and Composer Secret Manager Get are unit-proven so the live GCP pass does not invent identity or secrets mid-spend.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-30T11:35:21Z
- **Completed:** 2026-07-30T11:40:03Z
- **Tasks:** 2
- **Files modified:** 13

## Accomplishments

- Injectable WIF Ensure provisions pool, GitHub OIDC provider (`attribute.repository` + repo condition), CI SA, and `roles/iam.workloadIdentityUser` binding
- Bootstrap Details expose real `wif.provider` / pool / SA — `"deferred"` removed from ops/bootstrap
- Composer `gcp-secret-manager://` resolves through `AccessSecretVersion` with the same non-leaking failure path as AWS
- Act-only `gcp-wif-act-smoke.yml` + docs for Act/`gcloud` exchange without key material

## Task Commits

1. **Task 1: End-to-end WIF plan+Ensure + Composer SM Get** - `a08d7ea` (feat)
2. **Task 2: Act-only WIF smoke workflow + docs exchange path** - `4810f84` (docs)

**Plan metadata:** `e8c638f` (docs: complete plan)

## Files Created/Modified

- `internal/cloud/gcp/bootstrap/identity.go` — BuildIdentityPlan + Ensure
- `internal/cloud/gcp/bootstrap/wif_client.go` — WIFAPI + live IAM/CRM adapter
- `internal/cloud/gcp/bootstrap/identity_test.go` — fake-client unit proof
- `internal/cloud/gcp/secrets/secrets.go` — GetSecretValue / AccessSecretVersion
- `internal/cloud/gcp/ops/day2.go` — wire WIF into Bootstrap.Ensure Details
- `internal/cli/build.go` / `root.go` — Composer GCP case
- `.github/workflows/gcp-wif-act-smoke.yml` — Act-only keyless auth smoke
- `docs/gcp-experimental.md` / `docs/gcp-acceptance.md` — Act + gcloud path

## Decisions Made

- Repo attribute condition is exact `acourtiol/magelift` (no org wildcard)
- Provider resource names rewritten to project number for google-github-actions/auth
- Hosted required check intentionally omitted (minutes exhausted)

## Deviations from Plan

None - plan executed exactly as written.

## Threat Flags

None beyond plan threat model (WIF + Composer SM surfaces mitigated per T-07-01..04).

## Known Stubs

None.

## Self-Check: PASSED

- Found identity.go, wif_client.go, identity_test.go, secrets_test.go, gcp-wif-act-smoke.yml, 07-01-SUMMARY.md
- Found commits a08d7ea, 4810f84
