# Historical run records

Each record describes an earlier MageLift checkout, Magento version, and
configuration. Use the current [provider guide](../capability-matrix.md) to
choose a runtime and find the checks you can repeat today.

## AWS

- [ECS Fargate preview with Magento 2.4.9](aws-ecs-fargate-magento-live-20260813ai.md)
- [ECS Fargate with Aurora, Amazon MQ, and OpenSearch Serverless](aws-ecs-fargate-packed-keep-awsba-20260823.md)
- [ECS Fargate with provisioned OpenSearch](aws-ecs-fargate-packed-keep-mlaw1-20260915.md)
- [ECS Managed Instances](aws-ecs-managed-instances-packed-keep-awsmi-20260823.md)
- [EKS Auto Mode](aws-eks-auto-mode-packed-keep-awsek-20260823.md)
- [EKS with self-managed nodes](aws-eks-self-managed-magento-live-20260817bf.md)
- [Existing VPC and RDS](aws-brownfield-adopt-2026-08-02.md)
- [Aurora restore](aws-aurora-application-fixture-live-2026-08-17.md)
- [CloudFront custom domain HTTPS](aws-cloudfront-alias-https-live-2026-08-13.md)
- [CloudFront WAF rules for Magento](aws-cloudfront-magento-safe-waf-live-2026-08-13.md)

## Google Cloud

- [GKE Autopilot with Magento 2.4.9](gcp-gke-autopilot-magento-live-gcap28-20260820.md)
- [GKE Autopilot with retained resources](gcp-gke-autopilot-magento-live-gcap29-20260823.md)
- [GKE Autopilot deployment rehearsal](gcp-gke-autopilot-magento-live-mldp3-20260914.md)
- [GKE Autopilot search](gcp-gke-autopilot-magento-search-live-mldp6-20260914.md)
- [GKE Autopilot preview lifecycle](gcp-gke-autopilot-preview-loop-mldp7-20260915.md)
- [GKE Autopilot configuration from the alpha](gcp-autopilot-alpha-configuration.md)
- [GKE Standard deployment](gcp-gke-standard-magento-live-mldp4-20260914.md)
- [GKE Standard with Valkey 9](gcp-gke-standard-magento-valkey90-live-20260815.md)
- [GKE Standard, high availability](gcp-gke-ha-standard-magento-live-mldp5-20260914.md)
- [GKE Standard pod, node, and backing-VM failure checks](gcp-gke-ha-standard-magento-live-gcha36-20260820.md)
- [Cloud SQL restore](gcp-cloudsql-application-fixture-live-2026-08-15.md)
- [Operator commands and expired preview cleanup](gcp-operator-verbs-live-mldp8-20260915.md)
- [URL-map HTTPS failover](gcp-edge-failover-https-live-20260817bl.md)
- [Cloud Armor import attempt](gcp-edge-armor-attempt-20260813af.md)
- [Schema mismatch during rollback](gcp-gke-autopilot-schema-mismatch-gcap28-20260820.md)
- [HTTP 500 after an import](gcp-rc19-proof-http500-20260922.md)

## Other archived runs

- [Fastly domain, TLS, and purge](fastly-adapter-live-2026-08-13.md)
- [New Relic EU operations](newrelic-operations-live-2026-08-13.md)
- [OVHcloud MKS, infrastructure only](https://github.com/magelift/magelift/blob/main/docs/evidence/ovh-mks-infrastructure-live-2026-08-12.md)
- [Scaleway Kapsule, infrastructure only](https://github.com/magelift/magelift/blob/main/docs/evidence/scaleway-kapsule-infrastructure-live-2026-09-15.md)
- [Earlier release checks](release-checks-2026.md)

## Provenance of the operator-command record

The original 2026-09-15 seal of
`runs/gcp-operator-verbs-mldp8-20260915.sealed.jsonl` was voided on 2026-09-16.
The line-14 cleanup record verified. All 13 cell digests failed verification
and their original inputs could not be reconstructed. The independent
[mldp8 run record](gcp-operator-verbs-live-mldp8-20260915.md) corroborated the
artifact digest `sha256:8588b13f…fdb2be4`, the seed
`magento-249-sanitized-definer-free.sql.gz`, and the 13 PASS and 1 SKIP counts.
The bundle was then resealed. Only the 13 stale digests changed. The other
five sealed bundles verified during the review.
