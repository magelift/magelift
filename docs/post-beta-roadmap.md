# Post-beta roadmap

Explicitly **after** public beta. Do not block the certified AWS path on these.

| Track | Intent |
| --- | --- |
| Brownfield attach | Bring an existing VPC/DB into MageLift gradually; greenfield stays default |
| Dump / media cutover | Seed DB and media from PaaS dumps (Phase 5); config importers already ship as `magelift init --from-acc` / `--from-upsun` — see [migrating-from-paas.md](migrating-from-paas.md) |
| Pulumi Cloud / ESC | Optional hosted state + OIDC for agencies; DIY S3/GCS remains the OSS default |
| FinOps SaaS | Multi-project cost anomaly, rightsizing, drift — CLI keeps full power |
| Provider cost adapters | GCP/OVH/Scaleway/EKS `CostEstimator` implementations (AWS Fargate adapter exists); MI launchType pricing cells |
| Storefront recipes | Docs only (A1): wire Next/PWA to Magento outputs; no in-core OpenNext |
| GitHub org | Move from personal fork to `magelift` org when private quality bar is met |
| Signed remote plugins | Only after Sigstore allowlists; until then use compiled extension binaries |
| ECS RabbitMQ HA ladder | Adopt Chantelle pattern: EFS+AMQPS single-node → 3-node quorum on Managed Instances; keep Amazon MQ as optional expensive cell; never flip 1↔3 in place |
| ECS Managed Instances | Optional capacity provider beside Fargate: better $/vCPU with RI/SP once density is known; keep Fargate as the certified easy path. Gate behind catalog (e.g. `launchType: fargate\|managed-instances`) + acceptance |
| MageLift OpenSearch SigV4 data-plane | Paid AWS acceptance: index/query/reconnect/least-privilege on a real MageLift stack (deferred from public-tag gate; see [release-readiness.md](release-readiness.md)) |

See [capability matrix](capability-matrix.md) and [ADR 0007](adr/0007-multi-provider-community-targets.md).
