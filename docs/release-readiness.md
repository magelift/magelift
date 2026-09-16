# Public release readiness

No public tag may be created until maintainers have documented:

- trademark and package-name clearance for MageLift;
- availability and ownership of GitHub, Packagist, GHCR, and documentation names;
- complete dependency license and NOTICE review;
- signed release artifacts, checksums, SBOM, provenance, and vulnerability gates;
- green required CI and acceptance checks for the release scope;
- accurate non-affiliation language and descriptive trademark usage.

## OpenSearch gate (re-scoped 2026-07-22)

Managed OpenSearch remains **certified intent** with two Magento contracts:
provisioned domains use HTTPS:443 in-VPC with HTTP auth off and no sidecar;
AOSS uses a local SigV4 proxy because Magento cannot sign. The
**public-tag** gate no longer requires a paid MageLift OpenSearch acceptance
account. It closes with three substitutes:

- **MageLift offline wiring**: Pulumi/unit mocks for Magento search env (`TestRuntimeWiresMagentoOpenSearchEnvFromEndpoint`, `TestRuntimeWiresAOSSThroughSigningProxy`); Magento search env absent when the endpoint is empty
- **Free-tier AWS matrix**: Applied `searchMode: disabled` + queue cells; preview-only OpenSearch serverless plan (2026-07-21). Re-proven create-once + `db`/`ecs-rabbitmq`/`ecs-artemis` + kill/resume + dual assert_clean (2026-07-29). Evidence: committed samples under [evidence/](evidence/README.md); local re-runs under gitignored `.magelift/`
- **External ops precedent**: Prior Terraform Magento+OpenSearch work (ElasticSuite, no SigV4). See [sources/prior-terraform-opensearch.md](sources/prior-terraform-opensearch.md); private repos not named

That prior work proves Magento + AWS OpenSearch *ops*, not MageLift's live search
data plane. Do **not** claim "live Magento search on MageLift acceptance is
green" until a paid pass.

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
Floci AWS, floci-gcp, and mocks remain the default automated verification. They do not certify Autopilot, Armor, managed TLS, or Magento Cloud SQL PITR.

## Expanded RC1 scope, 2026-08-06

The expanded RC1 work deliberately separates a certified target from the much larger
architecture catalog. A live pass on one Magento 2.4.9 shape does not certify every
release, managed service, edge provider, or observability vendor. The current evidence
index records the exact tested shape and the remaining gates.

### RC1 verification and cleanup snapshot, 2026-08-07

The final local verification completed with the Go toolchain constrained to one
build or test process at a time: `go test ./...` passed 1,126 tests in 102
packages. `make php-test` passed Composer validation and audit, PHP static
analysis, Psalm, and 119 PHPUnit tests with 274 assertions. The acceptance shell
shape suite, generated-file checks, strict documentation build, and OpenSpec
validation also passed. The knowledge bundle conforms with zero errors and one
pre-existing broken-link warning.

The direct provider audit found no live MageLift runtime resources in the
audited AWS, GCP, Scaleway, or OVH inventories, and no marked disposable Fastly
or Cloudflare edge objects. The follow-up audit also removed two exact AWS
acceptance hosted zones, four stale Cloudflare delegation records, sixteen
orphaned RDS error log groups, and scheduled four AWS acceptance secrets for
deletion with a seven-day recovery window. AWS tagging tombstones are reported
separately from owning-service inventories.
The audit deliberately preserves the unprefixed GCP `magelift-composer-auth`
secret because ownership is unproven, records the AWS KMS key's mandatory
deletion date of 2026-08-14, and records the empty OVH project that still needs
OVH's email and password confirmation flow for project termination. Scaleway's
empty `MageLift` project is the account default, so its API refuses project
deletion; no billable resource remains in it.

The current GCP evidence covers three tested 2.4.9 runtime shapes: preview with
12 live cells, Standard with 13 live cells, and HA Standard with 13 application
cells plus a dedicated 14-cell pod-loss run, all using Cloud SQL MySQL 8.4. The current application records use
Memorystore `VALKEY_9_0`; the older 2026-08-07 HA record using Valkey 8.0 is
retained as historical evidence. The corrected adapter selects `VALKEY_9_0`
(GA), with explicit `VALKEY_9_1` (Preview) support, and the 2026-08-12
infrastructure-only probe proved the `VALKEY_9_0` API mapping, `ACTIVE` state,
and provider-reported multi-zone metadata with independent cleanup. The
current HA run closes one bounded application cell, while the broader
architecture, failure, recovery, and matrix evidence remain open. The 2.4.6
MySQL topology passes are retained as runtime evidence only because the dated
source does not list that combination. Current proof is the
[evidence pack](evidence/README.md).

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

## Gate board (2026-07-30, RELEASE-05 / D-05)

Every public-tag gate is **Closed**, **Deferred**, or **Offline closed** with a named reason. No undetermined rows.

| Gate | Status | Evidence |
| --- | Closed / Deferred / Pending | --- |
| Trademark / package-name clearance | **Closed** | MageLift / magelift.dev |
| Contract freeze (`v1.0.0-rc.1`) | **Closed** | [versioning.md](versioning.md); CHANGELOG `[Unreleased]` baseline |
| OpenSearch public-tag substitute | **Closed** | Offline SigV4 wiring + free-tier matrix + [prior Terraform ops](sources/prior-terraform-opensearch.md) (repos not named) |
| OpenSearch live SigV4 data-plane | **Deferred** | Post-tag / paid acceptance checklist above |
| Queue matrix (`ecs-rabbitmq` + experimental Artemis) | **Closed** | AWS preview `db` / `ecs-rabbitmq` / `ecs-artemis` PASS 2026-08-06 with direct cleanup; [evidence](evidence/aws-ecs-fargate-magento-live-20260813ai.md) |
| Measured time-to-preview | **Closed** | ~631s / 10m31s eu-north-1 |
| `/health` on MageLift runtime images | **Closed** | nginx + FrankenPHP short-circuit; `curl` in image; `scripts/image-health-test.sh`; Varnish pass-through |
| GCP Magento deploy Ops | **Closed for tested runtime shapes** | 2.4.6-p15 standard and HA are runtime evidence only because Cloud SQL MySQL is Adobe-unsupported for that patch; 2.4.9 preview, Standard, and current HA pass their bounded live stages with the current Valkey 9 cache intersection, but resilience and broader architecture evidence remain experimental |
| GCP certify (GCP-06 / multi-cloud claim) | **Autopilot certified for evidenced cells; Standard experimental** | Certified Magento origins are AWS ECS Fargate and GCP GKE Autopilot. GKE Standard 2.4.9 Valkey 9.0 and HA known-content (pod/node/backing-VM zone-loss) are experimental. Physical zone outage, regional DR, edge traffic, and the rest of the release matrix stay gated; [evidence](evidence/README.md); [ADR 0002](adr/0002-certified-vs-experimental.md) |
| GCP Magento 2.4.6-p15 standard live cell | **Runtime evidence only** | 13/13 stages PASS with pinned OpenSearch 3/RabbitMQ 4.3, full database dump, and independent cleanup, but Cloud SQL MySQL is unsupported by the Adobe 2.4.6-p15 row; [evidence](evidence/README.md) |
| GCP Magento 2.4.6-p15 HA Standard live cell | **Runtime evidence only** | 13/13 stages PASS with three-zone OpenSearch, two-node RabbitMQ, full database dump, and independent cleanup, but Cloud SQL MySQL is unsupported by the Adobe 2.4.6-p15 row; [evidence](evidence/README.md) |
| GCP Magento 2.4.9 standard live cell | **Experimental outside this exact bounded cell** | 13/13 cells PASS with Cloud SQL `MYSQL_8_4`, current OpenSearch 3/RabbitMQ 4.3, the real B2B dump, explicit `VALKEY_9_0`, and independent cleanup. The 2026-08-15 run repeats the earlier `20260813an` boundary; public HTTP/TLS, HA, DR, and broader architecture claims remain open; [evidence](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md) |
| GCP Magento 2.4.9 HA Standard live cell | **Experimental** | [gcha36](evidence/gcp-gke-ha-standard-magento-live-gcha36-20260820.md) catalog PASS including Magento seed-probe after pod, node, and backing-VM zone-loss. Not a certified target. Physical zone outage, regional DR, fencing/failback, and the broader matrix stay open. |
| NOTICE + license review | **Closed** | `NOTICE`, `LICENSE`, `make license-check` (recorded below) |
| First ship path | **Closed** | GitHub Release archives first; Homebrew cask publishes at the tag with the `cask-verify` pipeline job as the tested macOS install path; Windows = archive until winget/Scoop owned |
| Packaging smoke | **Closed** | 2026-09-15 - `make release-smoke` exit 0 (serial single-target host build, both binaries); 2026-09-13 same |
| Shared Kubernetes day-2 | **Offline closed** | Unit/fake-clientset: `*kube.Observe` + `*kube.Steps` type-identity across gcp/eksops/ovh/scaleway; AES256 DIY state unit proof. **Not** live GKE/OVH/SCW Magento acceptance beyond the GCP certified create-once path. |
| Cloudflare DNS cutover (MIGRATE-04) | **Closed** | Live `cutover:dns` PASS + `--cleanup` on an operator-owned preview host (2026-08-02); script + Zone.DNS Edit via `cf` CLI. Preview-host rehearsal, not a production storefront cutover. See [evidence](evidence/README.md). |
| Hosted CI / GitHub Actions | **Closed** | Public repo; hosted workflows run on `main`. Shell/docs/Go path filters green after org move (see Actions). Local Act remains available for offline iteration. |
| Brownfield attach (ATTACH-01..04) | **Closed** | Existing\|Adopt\|Refuse\|Detach covered offline + live AWS VPC+RDS adopt. Docs: [brownfield-attach.md](brownfield-attach.md) |
| Free-tier AWS VPC+RDS adopt confirm | **Closed** | Live PASS 2026-08-02. Preview ADOPT VPC+RDS, apply +74, destroy -74, describe-after-destroy VPC/RDS intact, then external cleanup. Evidence: [aws-brownfield-adopt](evidence/aws-brownfield-adopt-2026-08-02.md) |
| GCP preview, standard, and high-availability Magento 2.4.9 | **Experimental beyond the Autopilot certified preview cell** | Autopilot preview is certified ([gcap28](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md)). Standard and HA remain experimental ([20260815](evidence/gcp-gke-standard-magento-valkey90-live-20260815.md), [gcha36](evidence/gcp-gke-ha-standard-magento-live-gcha36-20260820.md)). Physical zone outage, recovery, edge, and the broader architecture matrix remain gates; `VALKEY_9_1` is Preview-only |
| Full Magento 2.4.6 through 2.4.9 architecture matrix | **Pending** | Compatibility catalog, cold baselines, warm groups, unsupported intersections, and blocked providers stay open. Current Magento proofs are in the [evidence pack](evidence/README.md) |
| OVHcloud / Scaleway full Magento certification | **Pending** | Scaleway has infrastructure/runtime smoke evidence; OVH MKS networking remains an adapter issue |
| Fastly cross-cloud edge adapter | **Implemented / Experimental** | `magelift edge plan/apply/destroy` is wired to the current Domain Management CLI, authenticated Fastly profiles, idempotent domains, purge policy, and exact marker cleanup. Live adapter lifecycle cleanup and routed-domain smoke pass; production edge certification remains deferred |
| Third-party observability provisioning | **Deferred / Experimental** | Datadog, New Relic, and OTLP intents are accepted by the provider-neutral boundary. The 2026-08-09 New Relic CLI marker/NRQL probe proves bounded data-plane delivery, the 2026-08-15 GKE Autopilot and GKE Standard Contrib cells prove two live collector paths with separate ingest/query credentials, logs/metrics/traces delivery, and direct cleanup, and the AWS ECS Contrib exercise proves sidecar rollout/configuration and eventual three-signal delivery. The AWS finite verifier window was exceeded before the trace became queryable, so broader collector delivery, credential rotation, retention/alerting, native composition, and provider-wide evidence remain open |

### Time-to-preview

```text
elapsed_seconds: 631
date: 2026-07-21T19:51Z (Pulumi create Duration 10m31s)
account: free-tier <redacted> / eu-north-1
shape: preview + rds-mysql + fck-nat + searchMode=disabled + queueMode=db (then ecs-rabbitmq / ecs-artemis on same stack)
```

Thorough free-tier matrix (same account, 2026-07-21): applied `db` / `ecs-rabbitmq` /
`ecs-artemis` with broker image verify; day-2 CLI mostly green (runtime Magento ops
were blocked by acceptance `/health` 404; fixed in MageLift runtime images);
preview-only `nat-gateway`, OpenSearch serverless, Aurora Serverless v2; guards
confirmed for OpenSearch provisioned + `amazon-mq`×preview (2-AZ vs 3-AZ). Committed
samples: [aws matrix](evidence/aws-ecs-fargate-magento-live-20260813ai.md),
[gcp matrix](evidence/gcp-gke-autopilot-magento-live-gcap28-20260820.md). Local re-runs write under
gitignored `.magelift/`.

### License checklist (pre-tag)

1. Root [`NOTICE`](../NOTICE) and [`LICENSE`](../LICENSE) present (Apache-2.0 + Adobe non-affiliation).
2. `make license-check` (or CI `go-licenses`) clean for disallowed types.
3. GoReleaser archives retain LICENSE + NOTICE (verified by `scripts/release-smoke-local.sh`).

Ship GitHub Release archives first after clearance. The cask install path is
tested on macOS by the release pipeline's `cask-verify` job (tap plus
install plus `magelift version` on `macos-latest`); keep that job green
before calling the cask done. Treat Windows as archive download until a
maintainer owns winget/Scoop if demand appears.

### Local packaging smoke

Run `./scripts/release-smoke-local.sh` or `make release-smoke` for a host-only
check (`goreleaser build --single-target --parallelism=1`, `GOMAXPROCS=1`,
`GOFLAGS=-p=1`). Do not use a full multi-platform `goreleaser release` as a local
smoke; that matrix belongs on CI.

### Packaging smoke record

- 2026-09-15: `make release-smoke` exit 0 (serial single-target host build; asserts `magelift` plus `magelift-provider-gcp` in `dist/`)
- 2026-09-13: `make release-smoke` exit 0 (serial single-target host build; asserts `magelift` plus `magelift-provider-gcp` in `dist/`)
- 2026-07-28: `make release-smoke` exit 0 (serial single-target host build, ~159s)
- 2026-07-22: `goreleaser check` validated `.goreleaser.yaml`

### License check record

- 2026-09-15: `make license-check` exit 0
- 2026-09-13: `make license-check` exit 0
- `make license-check` green on 2026-07-22 with `--ignore=github.com/ovh/pulumi-ovh`
  (Apache-2.0 at module root; recorded in `NOTICE`).
