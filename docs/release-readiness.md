# Public release readiness

No public tag may be created until maintainers have documented:

- trademark and package-name clearance for MageLift;
- availability and ownership of GitHub, Packagist, GHCR, and documentation names;
- complete dependency license and NOTICE review;
- signed release artifacts, checksums, SBOM, provenance, and vulnerability gates;
- green required CI and acceptance checks for the release scope;
- accurate non-affiliation language and descriptive trademark usage.

## OpenSearch gate (re-scoped 2026-07-22)

Managed OpenSearch remains **certified intent** (native Magento client + pinned
`aws-sigv4-proxy` sidecar). The **public-tag** gate no longer requires a paid
MageLift OpenSearch acceptance account. It closes with three substitutes:

- **MageLift offline wiring** — Pulumi/unit mocks for SigV4 sidecar + Magento env (`TestRuntimeAddsSigV4ProxyForMagentoOpenSearch` and related); proxy absent when `searchMode: disabled`
- **Free-tier AWS matrix** — Applied `searchMode: disabled` + queue cells; preview-only OpenSearch serverless plan (2026-07-21). Re-proven create-once + `db`/`ecs-rabbitmq`/`ecs-artemis` + kill/resume + dual assert_clean (2026-07-29). Evidence: `.magelift/matrix-results.md`; proof note `scratch/03-06-paid-proof.md`
- **Chantelle external ops** — Prior Terraform Magento+OpenSearch work at Chantelle (ElasticSuite, no SigV4) — see [sources/chantelle-opensearch.md](sources/chantelle-opensearch.md); private repos not named

**Honesty:** Chantelle proves Magento + AWS OpenSearch *ops*, not MageLift’s
SigV4 data plane. Do **not** claim “live Magento search on MageLift acceptance is
green” until a paid pass.

### Post-tag / paid-acceptance checklist (deferred)

When credits allow, prove on a real MageLift stack:

1. Index creation and catalog indexing
2. Storefront / Magento search queries
3. Reconnect after task recycle
4. Least-privilege task role (no wildcards beyond documented actions)

Floci does not substitute this data-plane checklist.

Real AWS acceptance (non-OpenSearch cells) remains a local maintainer activity.
Use [Local AWS acceptance](aws-acceptance.md) with the `preview` preset, destroy on
exit, and free credits carefully. There is no GitHub Actions cloud spend matrix;
Floci and mocks remain the default automated verification.

The release workflows use Release Please for Conventional Commit versioning and
GoReleaser for reproducible multi-platform archives, SHA-256 checksums, SBOMs, and
Homebrew cask publication. The release job verifies the keyless Sigstore bundle for
the checksum manifest before publishing its GitHub artifact attestation. The
container-image workflow publishes the Debian PHP and
FrankenPHP classic matrices to GHCR, attaches SBOM and SLSA provenance, and signs
and verifies each immutable digest with keyless Cosign. Configure `HOMEBREW_TAP_GITHUB_TOKEN`
only in the release environment; it is not a repository secret used by pull
requests. Configure `RELEASE_PLEASE_TOKEN` as a narrowly scoped GitHub App or
fine-grained token so release tags can trigger the artifact workflow; the default
`GITHUB_TOKEN` does not start a second workflow.

If clearance fails, rename every identifier before the first public tag.

## Gate board (2026-07-30 — RELEASE-05 / D-05)

Every public-tag gate is **Closed**, **Deferred**, or **Offline closed** with a named reason. No undetermined rows.

| Gate | Status | Evidence |
| --- | Closed / Deferred / Pending | --- |
| Trademark / package-name clearance | **Closed** | MageLift / magelift.com |
| Contract freeze (`v1.0.0-rc.1`) | **Closed** | [versioning.md](versioning.md); CHANGELOG `[Unreleased]` baseline |
| OpenSearch public-tag substitute | **Closed** | Offline SigV4 wiring + free-tier matrix + [prior Chantelle Terraform ops](sources/chantelle-opensearch.md) (repos not named) |
| OpenSearch live SigV4 data-plane | **Deferred** | Post-tag / paid acceptance checklist above |
| Queue matrix (`ecs-rabbitmq` + experimental Artemis) | **Closed** | Free-tier infra 2026-07-21; harness cell PASS 2026-07-29 |
| Measured time-to-preview | **Closed** | ~631s / 10m31s eu-north-1 |
| `/health` on MageLift runtime images | **Closed** | nginx + FrankenPHP short-circuit; `curl` in image; `scripts/image-health-test.sh`; Varnish pass-through |
| GCP Magento deploy Ops | **Closed** | Live `deploy:candidate` PASS 2026-08-02 — [gcp-matrix-results](evidence/gcp-matrix-results-2026-08-02.md) + [certified pass](evidence/gcp-certified-pass-2026-08-02.md) |
| GCP certify (GCP-06 / multi-cloud claim) | **Closed** | Certified `gcp`/`gke-autopilot` 2026-08-02 create-once (SC1–SC5 + dump + DNS); [evidence](evidence/README.md); ADR 0007 second first-party target |
| NOTICE + license review | **Closed** | `NOTICE`, `LICENSE`, `make license-check` (recorded below) |
| First ship path | **Closed** | GitHub Release archives first; Homebrew cask optional post-tag; Windows = archive until winget/Scoop owned |
| Packaging smoke | **Closed** | 2026-07-28T15:11:04Z — `release smoke ok binary=dist/magelift_darwin_arm64_v8.0/magelift (serial single-target)` via `make release-smoke` (`GOMAXPROCS=1`, `--parallelism=1`) |
| Shared Kubernetes day-2 | **Offline closed** | Unit/fake-clientset: `*kube.Observe` + `*kube.Steps` type-identity across gcp/eksops/ovh/scaleway; AES256 DIY state unit proof. **Not** live GKE/OVH/SCW Magento acceptance beyond the GCP certified create-once path. |
| Cloudflare DNS cutover (MIGRATE-04) | **Closed** | Live `cutover:dns` PASS + `--cleanup` on `magelift-preview.alexandrecourtiol.com` (2026-08-02); script + Zone.DNS Edit via `cf` CLI. Preview-host rehearsal — not a production storefront cutover. See [gcp-certified-pass](evidence/gcp-certified-pass-2026-08-02.md). |
| Hosted CI / GitHub Actions minutes | **Deferred** (Act-only) | Maintainer lock: hosted Actions minutes exhausted; local `make verify` + Act until minutes return — do not claim hosted CI green. See [lint-policy.md](lint-policy.md). |
| Brownfield attach (ATTACH-01..04) | **Closed** | Existing\|Adopt\|Refuse\|Detach covered offline + live AWS VPC+RDS adopt. Docs: [brownfield-attach.md](brownfield-attach.md) |
| Free-tier AWS VPC+RDS adopt confirm | **Closed** | Live PASS 2026-08-02 — preview ADOPT VPC+RDS, apply +74, destroy −74, describe-after-destroy VPC/RDS intact, then external cleanup. Evidence: [aws-brownfield-adopt](evidence/aws-brownfield-adopt-2026-08-02.md) |

### Time-to-preview

```text
elapsed_seconds: 631
date: 2026-07-21T19:51Z (Pulumi create Duration 10m31s)
account: free-tier 669890779205 / eu-north-1
shape: preview + rds-mysql + fck-nat + searchMode=disabled + queueMode=db (then ecs-rabbitmq / ecs-artemis on same stack)
```

Thorough free-tier matrix (same account, 2026-07-21): applied `db` / `ecs-rabbitmq` /
`ecs-artemis` with broker image verify; day-2 CLI mostly green (runtime Magento ops
were blocked by acceptance `/health` 404 — fixed in MageLift runtime images);
preview-only `nat-gateway`, OpenSearch serverless, Aurora Serverless v2; guards
confirmed for OpenSearch provisioned + `amazon-mq`×preview (2-AZ vs 3-AZ). Committed
samples: [aws matrix](evidence/aws-matrix-results-sample-2026-07-29.md),
[gcp matrix](evidence/gcp-matrix-results-2026-08-02.md). Local re-runs write under
gitignored `.magelift/`.

### License checklist (pre-tag)

1. Root [`NOTICE`](../NOTICE) and [`LICENSE`](../LICENSE) present (Apache-2.0 + Adobe non-affiliation).
2. `make license-check` (or CI `go-licenses`) clean for disallowed types.
3. GoReleaser archives retain LICENSE + NOTICE (verified by `scripts/release-smoke-local.sh`).

Ship GitHub Release archives first after clearance. Add brew only when the cask
install path is tested on macOS. Treat Windows as archive download until a
maintainer owns winget/Scoop if demand appears.

### Local packaging smoke (serial only)

On developer Macs run `./scripts/release-smoke-local.sh` or `make release-smoke` only.
It uses `goreleaser build --single-target --parallelism=1`, `GOMAXPROCS=1`, and
`GOFLAGS=-p=1`. Do **not** run a full multi-platform `goreleaser release` locally —
parallel cross-compiles have exhausted RAM/SWAP and caused kernel panics. Even a
serial host build of this Pulumi-linked binary can thrash under Cursor; if swap
climbs, abort and finish the smoke in a plain Terminal. Full matrices belong on CI
runners.

### Packaging smoke record

- 2026-07-28: `make release-smoke` exit 0 — `release smoke ok binary=dist/magelift_darwin_arm64_v8.0/magelift (serial single-target)` (~159s; goreleaser check + host darwin/arm64 snapshot). Maintainer allowed agent serial smoke under Cursor with abort-on-pressure; free pages stayed healthy.
- 2026-07-22: `goreleaser check` validated `.goreleaser.yaml`. Host
  `--single-target` build started compiling Pulumi SDKs, drove swap ~2.3 GB with
  near-zero free pages under Cursor — aborted to avoid another kernel panic.

### License check record

- `make license-check` green on 2026-07-22 with `--ignore=github.com/ovh/pulumi-ovh`
  (Apache-2.0 at module root; recorded in `NOTICE`).
