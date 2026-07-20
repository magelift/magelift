# MageLift documentation

Pre-alpha Magento deploy tooling: you own the cloud account; YAML and the CLI
drive builds, Pulumi, and day-2 ops.

| Topic | Doc |
| --- | --- |
| Product shape | [Architecture](architecture.md) |
| Contribute | [CONTRIBUTING](https://github.com/acourtiol/magelift/blob/main/CONTRIBUTING.md) |
| New cloud adapter | [Adding a provider](adding-a-provider.md) |
| CLI | [CLI reference](cli-reference.md) |
| AWS acceptance | [Local AWS acceptance](aws-acceptance.md) |
| Migrate from ACC / Platform.sh | [Migrating from PaaS](migrating-from-paas.md) |
| AWS EKS (experimental) | [AWS EKS experimental](aws-eks-experimental.md) |
| GCP (experimental) | [GCP experimental](gcp-experimental.md) |
| OVH (experimental) | [OVH experimental](ovh-experimental.md) |
| Scaleway (experimental) | [Scaleway experimental](scaleway-experimental.md) |
| Clean-room rules | [Provenance](provenance.md) |

Certified target: AWS ECS Fargate. Experimental: GCP GKE Autopilot, OVH MKS,
Scaleway Kapsule. Multi-cloud
is not claimed until two targets are certified (ADR 0007).

Independent of Adobe Inc. Product names are descriptive only.
