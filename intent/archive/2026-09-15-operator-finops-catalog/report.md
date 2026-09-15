# Report: operator-finops-catalog (order 12)

## Verdict

Pass. Five verbs live-proved on GCP (`mldp8`, 13/13 cells, zero
leftovers), AWS cost reads proven creds-only, four defects fixed with
tests, docs plus skill synced.

## What shipped

- Destroy-after-expiry fix in all five stack packages: the
  allow-expired decision now rides the spec into the Pulumi program,
  which skips only the expiry check. Expired preview deploys refuse,
  destroys proceed. Per-provider tests pin both sides.
- `isOnlyExpirationError` over-forgiveness removed: allow-expired
  planning used a substring check that forgave every defect when
  expiry was present. Replaced with validator selection; helpers
  deleted.
- AWS `cost --live` repaired: five Price List filter defects (EU
  location names, Fargate attribute-less products, three wrong
  product families, two wrong usagetype substrings, MQ `mq.` prefix).
  Live on eu-west-3: Fargate vCPU $17.74, memory $3.87, Valkey
  $10.51/month.
- AWS `cost --budget` repaired: notifications page size 1000 exceeds
  the API max of 100. Preview reports not-configured; staging class
  lists the five account budgets with amounts and the no-ownership
  notice.
- GCP cleanup provider wired into the CLI (the hook existed, no
  binary populated it). `cleanup plan|reconcile` now run live Cloud
  SQL inventory over ADC.
- Docs: `docs/operations.md` operator-verbs section,
  `magelift-operate` skill commands plus rules, evidence md plus
  sealed bundle, README row, regenerated manifest.

## Deviations

- First `mldp8 up` attempt inherited a stale OVH cell catalog from the
  operator shell and failed closed with zero cloud changes; purged,
  checkpoint entry removed, resumed green. Documented in the evidence.
- `cleanup reconcile` live-proved the full loop (claim, record, plan
  over live inventory, reconcile to `complete`) against a gone
  identity; a live Cloud SQL delete was not justified for a probe and
  stays unit-covered, stated in non-claims.
- RDS / OpenSearch / MQ live-price shapes fixed from API evidence but
  await a session that runs them; stated in non-claims.
- The service-networking peering refused deletion again (GCP-side);
  deleting the VPC removed it. Zero `mldp8` leftovers, cleaner than
  `mldp7`.

## Evidence

- `docs/evidence/gcp-operator-verbs-live-mldp8-20260915.md` plus
  `runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` (13 PASS, 1
  honest SKIP, scanned clean)
- Unit: 232 (5 stack pkgs) + 22 (pricing/cost) + 323 (cli/gcp-cost)
  green; Floci AWS + GCP suites green; docs build green

## Spend

GCP paid resources ~75 min, no leftovers. AWS Price List plus Budgets
reads only (no stack).
