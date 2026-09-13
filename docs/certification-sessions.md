# Packed live certification

Live Magento create/destroy is the slow path. Run **packed sessions**, not one
stack per checkbox. See [ADR 0010](adr/0010-live-certification.md).

Prove locally first (`make local-gates`: Pulumi mocks, offline harness, Floci
AWS, floci-gcp). Use a live cloud only when E2E is required. GCP is the
thorough Magento path. AWS, OVH, Scaleway, New Relic, SendGrid, Fastly, and
Cloudflare are light smoke: one bounded cell, then destroy. Do not KEEP those
stacks for dump-retrieve, collectors, or edge extras when GCP already has the
Magento-wired proof.

Overlap GCP and AWS waits with **separate git worktrees**, not two Magento
shops in one bill. Each tree needs its own prefix
(`MAGELIFT_GCP_ACCEPTANCE_NAME`, `MAGELIFT_AWS_ACCEPTANCE_PROJECT_TAG`,
OVH/Scaleway `MAGELIFT_*_ACCEPTANCE_PREFIX`), `GOMAXPROCS=1 GOFLAGS=-p=1`,
and Pulumi state that is not inherited from the sibling tree. Shared
prefixes such as `mlacc` are refused. The harness library is
`scripts/acceptance/lib-campaign-isolation.sh`.

Destroy on exit. Do not set `MAGELIFT_*_ACCEPTANCE_KEEP=true` unless you want a
retained debug cell. Load `magelift-certify` for live cells. Publish one current
proof file under [evidence](evidence/README.md); do not append a session ledger.

Floci AWS (`make floci-test-aws`) and floci-gcp (`make floci-gcp-test`) already
run in CI. Emulators do not certify Autopilot, Memorystore, Cloud Armor,
managed TLS, or Magento Cloud SQL PITR.
