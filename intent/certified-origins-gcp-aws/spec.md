---
status: specified
slug: certified-origins-gcp-aws
intent: intent.md
---

# Spec: two certified origins proved on a tight testing budget

Auto-approved per the standing `/goal` instruction. Defaults taken:

- GCP proves preview (full Magento), standard (deploy plus health),
  and HA (deploy plus health) at minimum sizes; AWS proves preview
  only, standard/HA stay documented experimental on AWS (thin
  credits, and GCP already covers the topology).
- AWS origin session cap $25, search cell $5 per ADR 0012, total AWS
  Phase 2 cap $30 against ~$180 credits. One retry inside the cap;
  beyond that AWS stays experimental and GCP carries v1.
- AWS region eu-west-3 (matches fixtures, EU audience).
- Live Dial proof uses an ephemeral CI-built bundle: throwaway
  `v0.0.0-dialproof.N` tag, download the signed provider bundle
  plus binary, install beside the CLI, run the GCP preview through
  Dial, destroy, delete tag and release. This also pre-validates
  the release pipeline before rc.1.

## Requirements

### Requirement: pyramid base green before spend

`make local-gates` (Pulumi mocks, harness, Floci AWS, floci-gcp)
SHALL pass with no account before any packed session opens.

#### Scenario: gates green

- **WHEN** local-gates runs
- **THEN** all four stages exit 0 and the log names each stage

### Requirement: GCP packed session certifies the reference origin

One packed GCP session SHALL prove preview (full Magento deploy,
health, reindex probe), standard (deploy plus health), and HA
(deploy plus health) on GKE Autopilot at minimum sizes, then
destroy everything and pass `assert_clean`, with spend recorded.

#### Scenario: reference origin green

- **WHEN** the session closes
- **THEN** evidence records each cell plus destroy plus
  assert_clean, and zero `magelift` resources remain

### Requirement: live Dial proof rides the GCP session

The GCP preview cell SHALL run at least preview-plus-deploy through
a Dialed subprocess using the ephemeral CI bundle, proving
trust-before-Dial end to end on live infrastructure.

#### Scenario: subprocess deploys live

- **WHEN** the preview deploy runs with the verified bundle
  installed
- **THEN** the CLI reports subprocess mode and the deploy succeeds

### Requirement: AWS packed session certifies the second origin

One packed AWS session SHALL prove preview create-once plus warm
catalog transitions on one digest at minimum sizes, then destroy
and pass `assert_clean`, with spend inside the $25 session cap.

#### Scenario: second origin green

- **WHEN** the session closes
- **THEN** evidence records create plus transitions plus destroy
  plus assert_clean, spend is inside cap, and the account holds
  zero leftovers

### Requirement: search and preview loops attach, never standalone

The order-9 search cells and order-10 preview loop SHALL run inside
these packed sessions (GCP session for GCP cells and the preview
loop, AWS session for the AWS search cell).

#### Scenario: no standalone stacks

- **WHEN** Phase 2 closes
- **THEN** no search or preview stack ever existed outside the two
  packed sessions

## Design

Session mechanics follow `docs/gcp-acceptance.md` and
`docs/aws-acceptance.md`: disposable project/account, TTL watchdog,
EXIT-trap destroy, six-hour default TTL, Budget alarms ($5/$25 on
AWS) before first create. Evidence lands under `docs/evidence/`
with spend lines. The ephemeral tag is pushed, consumed, and
deleted inside the GCP session window.

## Gotchas / policy flags

- Never interrupt mid-create/destroy; orphan ALBs/ENIs block VPC
  delete for a long time.
- Orphaned preview locks and `pending_operations` clear only after
  the account proves empty.
- No KEEP except a named packed GCP session with a written reason;
  default is destroy on EXIT.
- AWS never re-proves vendor cells (Fastly, New Relic, SendGrid,
  Cloudflare); those attach to GCP or stay unproven.
- GCP auth is currently expired and the target project is
  unconfirmed: both need the maintainer before the GCP session.
  RESOLVED 2026-09-13: maintainer re-authed; project is
  `digital-lab-341608` (billing enabled, no magelift leftovers;
  pre-existing `default`/`dev-vpc` networks and `dev-vpc-ip` address
  are not ours and stay untouched).

## Open questions carried forward

None. Both intent questions are decided above.
