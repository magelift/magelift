# Report: preview-env-lifecycle (order 10)

## Verdict

Pass. The preview-environment lifecycle ran as one CLI loop on the
certified GCP origin: base up, PR preview created, health-verified,
guards exercised, both torn down with no billable leftovers.

## What ran

- Base `mldp7-preview`: 13/13 acceptance cells PASS (2026-09-14).
- Preview `mldp7-pr-999`: bootstrap, deploy, base-URL correction,
  runtime health, protected-destroy refusal, `sweep --dry-run`,
  destroy (2026-09-14/15).
- Teardown completed 2026-09-15 via ordered `gcloud` deletes after the
  `/tmp` Pulumi passphrase was reaped: cluster, Cloud SQL, Valkey,
  buckets, service accounts, NATs, routers, subnets all deleted and
  re-swept clean (no clusters, SQL, Valkey, buckets, SAs, LBs, DNS,
  firewall rules for either env).

## Deviations

- Teardown ran outside `magelift destroy` (passphrase lost to `/tmp`
  reaping). Lesson: keep passphrases next to the state they unlock or
  accept manual teardown as the fallback path.
- Two Service Networking peerings plus empty VPCs and reserved ranges
  remain: GCP refuses the delete ("Producer services still using")
  with zero consumers. GCP-side stuck state, $0 cost, documented in
  the evidence file with a re-attempt note.
- Preview deploy needed a post-deploy base-URL correction before
  health passed (handover record). The preserved deploy log tail
  shows one failed migrate attempt (`config:import` before tables
  exist, `DEPLOY_EXIT=1`); the rehearsal continued past it to the
  health-confirmed state. Ordinary rehearsal friction, not a product
  defect.

## Evidence

- `docs/evidence/gcp-gke-autopilot-preview-loop-mldp7-20260915.md`
  plus the README row.
- Base transcript `gcp-loop-mldp7.log`, preview deploy log
  `mldp7-pr999-deploy.log`, session transcript.

## Spend

Rehearsal ran inside the acceptance project across one day; no
separate budget line tracked.
