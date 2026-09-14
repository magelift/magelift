---
status: planned
slug: certified-origins-gcp-aws
spec: spec.md
---

# Plan: two certified origins proved on a tight testing budget

Auto-approved per the standing `/goal` instruction.

## Files that change

- Evidence: NEW `docs/evidence/*` session records (GCP plus AWS).
- EDIT `docs/capability-matrix.md`: only if a cell status changes.
- EDIT `docs/evidence/README.md`: index the new records.
- Code: only if a live run exposes a defect (fix plus test, logged
  as Deviation).

## Order of work

- [x] 1.1 Pyramid base — verify: `make local-gates` exit 0
- [x] 1.2 Maintainer unblock: gcloud re-auth plus project confirm
  plus ephemeral-tag permission — verify: `gcloud` identity works
  and the project is named in writing
- [x] 1.3 Ephemeral CI bundle (`v0.0.0-dialproof.N`) — verify:
  workflow green, bundle plus binary downloaded, tag deleted after
  use (`.12` slim green, triple consumed by `mldp3`, tags
  `.8`/`.9`/`.10`/`.11`/`.12` deleted; evidence
  `gcp-gke-autopilot-magento-live-mldp3-20260914.md`)
- [ ] 1.4 GCP packed session: preview, standard, HA, live Dial
  proof, order-9 GCP search cell, order-10 preview loop —
  verify: evidence plus destroy plus assert_clean plus spend line
  (preview `mldp3` GREEN via subprocess, evidence committed;
  standard `mldp3` lost to transient GCP Valkey capacity, retry
  `mldp4` GREEN, evidence committed; HA `mldp5` 16/16 GREEN,
  evidence committed. Order-9/10 live runs attach here as order-8
  legs with shared evidence; their intents formalize at Phase 2.)
- [ ] 1.5 AWS packed session: preview create-once plus warm
  transitions (queue db, ecs-rabbitmq, amazon-mq; search
  disabled then provisioned per ADR 0012; database rds-mysql
  then aurora-mysql; ha multi-az; NO serverless apply,
  artemis, or CloudFront — August evidence stands), order-9
  AWS search cell — verify: evidence plus destroy plus
  assert_clean, spend inside $25
- [ ] 1.6 Matrix plus evidence index update — verify: docs build

## Risks

- GCP auth/project unconfirmed: session cannot start without the
  maintainer (box 1.2 is a question, not work).
- AWS retry margin: one retry inside the cap, then AWS goes
  experimental honestly.
- Long sessions must not be interrupted mid-create/destroy.

## Proof

Gate log, session transcripts, evidence files, spend lines,
assert_clean outputs.

## Handover note (2026-09-14, WSL session)

- 1.3: release triple PROVEN on `.8` (binary digest matches lock,
  `cosign verify-blob` OK, CLI loads `mode: subprocess`). Found
  and fixed a real bug: `cosignBlobVerifier` swapped
  bundle/binary (commit `4a1ad48`, regression test added). The
  `.8`/`.9` CLIs carry the bug; session CLIs are built from the
  tree. Workflow-green proof rides `.10` (in flight).
- 1.4: preview blocked at `day2:exec`: `Pulumi output "kubeconfig"
  must be a non-empty string` via the subprocess path. Next step:
  find whether exec should stay in-process (docs say day-2 ports
  stay in-process) or the Execute RPC must plumb kubeconfig.
  Logs: `/tmp/gcp-preview*.log` (this box only).

## Handover note 2 (2026-09-14, devbox session)

- 1.3: `.10` FAILED the same lockfile step as `.9` (`missing
  bundle`, run `34841348135`). Root cause: binary-format
  archives are not copied to `dist/`; bundles live in the
  per-target build dir. Fixed in `a566df3` (lockfile plus
  verify steps resolve paths via `dist/artifacts.json`; both
  proven against a local snapshot `dist/`). Retagged `.11`
  from `a566df3` (run `34854228510`); `.11` also carries the
  Execute fix, so it is the consumable Dial triple.
- 1.4: blocker FIXED in `e423031` (`outputs` op serves
  decrypted for day-2; new `redacted-outputs` op for
  display; ADR 0011 plus provider guide updated). local-gates
  green on devbox. Seed regenerated
  (`/tmp/magento-249-sanitized-definer-free.sql.gz`, validator
  PASS). Digest `8588b13…` verify OK (identity
  `devops@…iam.gserviceaccount.com`); no fresh sign needed.
  Session CLI pre-built at `/tmp/magelift-gcp-mldp3/magelift`.
- `.8`/`.9`/`.10` tags stay until `.11` goes green and its
  triple is consumed, then delete all four per box 1.3.
- Update 2 (devbox, ~15:30Z): `.11` cancelled for speed;
  slim dialproof config in `5c13afe` (linux/amd64, ~15 min).
  `.12` GREEN after two infra flakes (cosign 504, Go proxy
  read); triple installed beside
  `/tmp/magelift-gcp-mldp3/magelift` (`mode: subprocess`,
  digest matches lock). Dead tags `.8`/`.9`/`.10`/`.11`
  deleted (releases plus tags); `.12` stays until the
  preview run consumes it. Preview `up` launched as `mldp3`
  (fresh DIR); log shows `using subprocess provider
  magelift-provider-gcp v0.0.0-dialproof.12`.
- Names `mldp`, `mldp2` are spent (WIF pools tombstoned 30d);
  next preview run needs a fresh `MAGELIFT_GCP_ACCEPTANCE_NAME`
  (suggest `mldp3`) and a fresh DIR. Seed dump plus crypt key
  live in `/tmp` on this box (NOT in the repo): regenerate via
  the recipe in the session transcript or copy them over.
- Account `digital-lab-341608` verified empty (clusters, SQL,
  nets, secrets, buckets, pools all zero) at handover.
- `.8`/`.9` ephemeral tags stay until `.10` goes green, then
  delete all three per box 1.3.
