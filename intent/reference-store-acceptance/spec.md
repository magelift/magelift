# Spec: reference-store acceptance

## Requirements

### R1 — Pinned alpha recipe

One recipe, no ranges: Magento Open Source 2.4.9, PHP 8.5,
Cloud SQL MySQL 8.4, Valkey 9.0, recipe OpenSearch on GKE,
database queue, GCS media, GCP GKE Autopilot in one region,
preview preset as the loop environment. Scale bounds written
down (replicas, tiers, single region). Excluded promises
named: arbitrary versions, zero-downtime incompatible
schema changes, cross-cloud DR, hard spending cap.
Published as `docs/alpha-recipe.md`, the single pin sheet
the loop and the pilots share.

### R2 — Shipped-path loop on the RC

The loop runs on the release candidate tag
(`v0.1.0-alpha.1-rc.1`, kept, ask-first approved) and its
verified artifacts: installer, core upgrade path, provider
download. No dev builds, no old commits. Phases in order,
each with pass criteria and evidence rows:

1. Install RC via `install.sh`, `magelift version` matches.
2. `magelift build` produces the immutable Magento digest;
   one digest per phase, recorded.
3. Initial deploy of the recipe; verify per surface (HTTPS,
   assets, search, cron, SMTP, media) per the onboarding
   procedures.
4. Application release to a second digest; intended rollout
   plus bounded readiness.
5. Failed release: diagnosis from CLI output only, recovery
   via CLI verbs (no maintainer-only commands), shop
   healthy after.
6. Operation after credential expiry: expire the session
   credential, prove re-auth plus continued ops via CLI.
7. Backup (`env dump`, `env media-sync`, encryption key
   reference intact) and restore into a fresh environment;
   restored shop serves, admin login works, crypt key
   matches the secret reference.
8. Preview expiry: sweep destroys the expired preview and
   reports residual cost; orphan assertion clean; destroy
   on exit; per-session spend recorded.

### R3 — Residual-cost report

Sweep entries gain a top-level `residual` block hoisting the
destroy result's retained backups with honest framing: what
survived, why (retention), until when, and that amounts come
from provider billing (never estimated here). Unknown stays
visible (`residualUnknown`), never silent. Unit-tested;
proved live in phase 8.

### R4 — Evidence pack

One proof file under `docs/evidence/` following the cell
format (scope table with project redacted, catalog with
pass/fail per phase, spend, leftover), plus sealed JSONL
under `docs/evidence/runs/`. No secret values, ever. The
capability matrix stays unchanged unless the run reveals a
promise gap, which files back instead of stretching a row.

### R5 — Runbooks

Deploy, failed-release recovery, backup/restore, and expiry
runbooks land as sections in `docs/operations.md`, written
from the commands actually run. No procedure the loop did
not execute.

### R6 — Go/no-go plus tag command

The report ends with an explicit verdict and the exact
alpha tag command (`v0.1.0-alpha.1` from the proved
commit). This intent never pushes the tag.

### R7 — Pilot intake format

Time-to-working-shop, time-to-recover, restore success,
and actual monthly cost, recorded per pilot in a fixed
table. The format is defined in the report; intake itself
begins after the alpha tag.

### R0 — First release reads as first version

Nothing ever shipped, so nothing is version two. Before the RC
tag, host-side pre-release numbering is renumbered to first
version: lockfile schema 2 becomes 1, the `magelift-v2`
protocol marker becomes `magelift-v1`, `DialV2` becomes the
unversioned `Dial` (it is the only dial path). No migration
and no compat shims: there are no users to migrate. Third-party
versions (Go modules, toolchains, cloud APIs) and generic
semver prose are untouched, as are historical records under
`intent/archive/` and `docs/evidence/`.

## Design notes

- Live account: the disposable GCP acceptance project,
  confirmed by the maintainer before the first live
  command. Destroy on exit every session; no KEEP unless a
  retained debug cell is explicitly approved.
- New catalog `scripts/acceptance/cells-gcp-store-loop.txt`
  names the loop phases for the harness; the preview
  catalog is reused, not forked, for phase 3 surfaces.
- Cost honesty throughout: estimates, unpriced lists,
  per-session spend notes. Budgets alert, never cap.
- Pre-live gates (all ask): acceptance account
  confirmation, RC tag approval, spend acknowledgment.
  The tag command in R6 needs its own later approval.
- Out of scope: AWS parity, EU providers, new adapters
  or cells beyond the loop catalog, v1 freeze, matrix
  recertification.
