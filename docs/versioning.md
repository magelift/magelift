# Versioning and contract freeze

MageLift uses [Semantic Versioning](https://semver.org/) from the first public
tag, `v1.0.0-rc.1`. Treat this document as the RC freeze surface once that tag
ships. `1.0.0-rc.1` is a SemVer pre-release — it does not claim full `1.0.0`
compatibility; see the RC stability statement below for what is locked vs
reserved.

## RC freeze surface

Breaking changes to any of the following require a documented RC break note
(and, after `1.0.0`, a major bump):

| Surface | Frozen meaning |
| --- | --- |
| `magelift.yaml` `schemaVersion` | Additive fields OK; removals/renames bump |
| CLI command names | `deploy`, `preview`, `destroy`, day-2 verbs stay |
| Exit codes | Documented CLI exit codes stay stable |
| AWS ECS Fargate certified cells | See [capability-matrix.md](capability-matrix.md); honesty labels for OpenSearch |
| Provider IDs | `aws` / `gcp` / `ovh` / `scaleway` string IDs |

Experimental cells and providers may still return `ErrNotSupported` or change
without a bump when clearly labeled experimental.

## RC stability statement (`sdk/v1` and `platform.StackModule`)

`v1.0.0-rc.1` is a SemVer pre-release. The following are **stable during the RC
series** unless a documented RC break note lands in CHANGELOG / this document:

| Surface | Stable meaning |
| --- | --- |
| `magelift.yaml` `schemaVersion` | Additive fields OK; removals/renames need an RC break note |
| CLI verb names | `deploy`, `preview`, `destroy`, day-2 verbs stay |
| Documented exit codes | Stay stable |
| Provider ID strings | `aws` / `gcp` / `ovh` / `scaleway` |
| Certified AWS ECS honesty labels | OpenSearch / free-tier labels in the capability matrix |
| `sdk/v1` | `Target` / `Capability` / `Hook` shapes stay |
| `platform.StackModule` | Module registration and core stack-module contract stay, subject to the Phase 6 reservation below |

**May change during RC** without a major `v2` bump when labeled experimental:

- Experimental providers and experimental capability-matrix cells returning
  `ErrNotSupported`
- Day-2 port shapes on Kubernetes-shaped modules pending Phase 6

**Phase 6 reservation (explicit):** ROADMAP Phase 6 consolidates shared-Kubernetes
Observe and `deploy.Steps` into `internal/cloud/kube`. That work **may change
`platform` port wiring** for EKS / GKE / MKS / Kapsule modules during the RC
series without waiting for a major `v2` bump. Callers must not treat those
kube-shaped `platform` ports as frozen for the full RC series.

## OpenSearch honesty

- **Certified wiring:** SigV4 proxy sidecar + Magento env (offline mocks).
- **Certified free-tier cell:** `searchMode: disabled`.
- **Not claimed green on MageLift acceptance:** live Magento search data-plane
  (index/query/reconnect/IAM) until a paid pass — see [release-readiness.md](release-readiness.md).
- External ops evidence: prior Terraform Magento+OpenSearch work at Chantelle
  ([sources/chantelle-opensearch.md](sources/chantelle-opensearch.md)); private
  employer repos are not named.

## Changelog

Keep `CHANGELOG.md` (repository root) current under `[Unreleased]` until the first
Release Please tag. After `v1.0.0-rc.1`, Conventional Commits drive release notes.

## First public tag criteria

See the gate board in [release-readiness.md](release-readiness.md). First ship
path is GitHub Release archives; Homebrew is optional post-tag. First public
tag is `v1.0.0-rc.1`.
