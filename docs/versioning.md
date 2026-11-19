# Versioning and contract freeze

MageLift uses [Semantic Versioning](https://semver.org/) for releases. The proposed first public tag is `v1.0.0-rc.1`. The following commitments take effect when it ships.

## RC freeze surface

Breaking changes to any of the following require a documented RC break note
(and, after `1.0.0`, a major bump):

| Surface | Frozen meaning |
| --- | --- |
| `magelift.php` `schemaVersion` | Additive fields OK; removals/renames bump |
| CLI command names | `deploy`, `preview`, `destroy`, day-2 verbs stay |
| Exit codes | Documented CLI exit codes stay stable |
| AWS ECS Fargate cells | See [capability-matrix.md](capability-matrix.md); recorded coverage for OpenSearch |
| Provider IDs | `aws`, `gcp`, `azure`, `ovh` and `scaleway` string IDs |
| Provider lockfile schema | `magelift.providers.lock` `schemaVersion: 1`, SDK API `"v1"` |
| Provider `Execute` RPC | Operation names and result shape stay (including `validate-config`, `plan`, `preview`, `apply`, `destroy` and `outputs`) |

Experimental cells and providers may still return `ErrNotSupported` or change
without a bump when clearly labeled experimental.

## RC stability statement (`sdk` and `platform.StackModule`)

`v1.0.0-rc.1` is a SemVer pre-release. The following are **stable during the RC
series** unless a documented RC break note lands in CHANGELOG / this document:

| Surface | Stable meaning |
| --- | --- |
| `magelift.php` `schemaVersion` | Additive fields OK; removals/renames need an RC break note |
| CLI verb names | `deploy`, `preview`, `destroy`, day-2 verbs stay |
| Documented exit codes | Stay stable |
| Provider ID strings | `aws`, `gcp`, `azure`, `ovh` and `scaleway` |
| Archived AWS ECS test results | OpenSearch / free-tier labels in the capability matrix |
| `sdk` | `Target` / `Capability` / `Hook` shapes stay |
| `platform.StackModule` | Module registration and core stack-module contract stay, subject to the shared-Kubernetes reservation below |
| Provider extension boundary | Lockfile schema `1`, SDK API `"v1"`, `Execute` operation names and result shape stay |

**May change during RC** without a major `v2` bump when labeled experimental:

- Experimental providers and experimental capability-matrix cells returning
  `ErrNotSupported`
- Day-2 port shapes on Kubernetes-shaped modules while shared kube Observe/Steps
  consolidate under `internal/cloud/kube`

**Shared Kubernetes reservation (explicit):** consolidating Observe and
`deploy.Steps` into `internal/cloud/kube` **may change `platform` port wiring**
for EKS / GKE / MKS / Kapsule modules during the RC series without waiting for a
major `v2` bump. Callers must not treat those kube-shaped `platform` ports as
frozen for the full RC series.

## OpenSearch coverage

- Pulumi mocks check the SigV4 proxy sidecar and the Magento environment settings.
- The historical free-tier run used `searchMode: disabled`.
- Live Magento index, query, reconnect and IAM coverage is recorded in [release-readiness.md](release-readiness.md).
- External ops evidence: prior Terraform Magento+OpenSearch work
  [Prior OpenSearch deployment](sources/prior-terraform-opensearch.md)

## Changelog

Keep `CHANGELOG.md` (repository root) current under `[Unreleased]` until the first
Release Please tag. After `v1.0.0-rc.1`, Conventional Commits drive release notes.

## First public tag criteria

See the gate board in [release-readiness.md](release-readiness.md). First ship
path is GitHub Release archives; Homebrew is optional post-tag. First public
tag is `v1.0.0-rc.1`.
