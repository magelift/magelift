# MageLift documentation

Deploy Magento in your own cloud with ACC/Upsun-shaped YAML and CLI — without
renting a PaaS. First public tag target: `v1.0.0-rc.1` (see
[versioning](versioning.md)).

## First hour

1. [Getting started](getting-started.md) — install → sample config → preview
2. [Local vs cloud](local-vs-cloud.md)
3. [Sample shop](getting-started.md#3-sample-shop-config) (`examples/sample-shop` in the repo)
4. [Leave PaaS in a weekend](weekend-migrate.md)
5. [Compare to ACC / Upsun](compare-paas.md)

| Topic | Doc |
| --- | --- |
| Product shape | [Architecture](architecture.md) |
| What is certified | [Capability matrix](capability-matrix.md) |
| Release gates | [Release readiness](release-readiness.md) |
| Contribute | [CONTRIBUTING](https://github.com/acourtiol/magelift/blob/main/CONTRIBUTING.md) |
| New cloud adapter | [Adding a provider](adding-a-provider.md) |
| CLI | [CLI reference](cli-reference.md) |
| AWS acceptance | [Local AWS acceptance](aws-acceptance.md) |
| GCP acceptance | [Local GCP acceptance](gcp-acceptance.md) |
| Migrate from ACC / Platform.sh | [Migrating from PaaS](migrating-from-paas.md) |
| Existing VPC / RDS attach | [Brownfield attach](brownfield-attach.md) |
| Storefront recipes | [Storefront recipes](storefront-recipes.md) |
| Publishing | [Publishing / Community Launch](publishing.md) |
| Clean-room rules | [Provenance](provenance.md) |

Certified targets: AWS ECS Fargate and GCP GKE Autopilot. Experimental: OVH MKS,
Scaleway Kapsule, AWS EKS. See [capability matrix](capability-matrix.md).

Independent of Adobe Inc. Product names are descriptive only.
