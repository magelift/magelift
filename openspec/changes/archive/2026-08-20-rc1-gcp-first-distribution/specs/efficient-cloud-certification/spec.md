## ADDED Requirements

### Requirement: Packed warm sessions cover many cells per origin

Live certification MUST reuse one KEEP Magento stack per origin session. Local CLI contracts that session 1 will exercise (named health layers, dump-from-live, rollback-vs-schema fail-closed, SendGrid secret-reference validation on the environment config) MUST exist with unit or fake-client proof before KEEP is set. Session 1 MUST be GCP Autopilot Magento 2.4.9 preview and MUST run remaining Magento-wired observability, New Relic, Fastly, Cloudflare DNS, SendGrid delivery or typed unsupported, dump-retrieve, layered health, rollback-vs-schema, remaining HA or backup cells that still need Magento, and the existing failed-deployment injector by setting `MAGELIFT_GCP_FAILED_DEPLOY_DIGEST`, before a single destroy and `assert_clean`. Session 1 MUST NOT re-implement `InjectFailedDeployment`. Session 2 MUST be AWS ECS Fargate Magento and MUST NOT re-prove Fastly, New Relic, SendGrid, or Cloudflare. Session 3 MAY run OVHcloud and Scaleway infrastructure-only cells concurrently with session 2 when credentials, state, and cleanup scopes are isolated. Magento on OVHcloud and Scaleway MUST remain experimental. GCS, Secret Manager, Pub/Sub, Logging, and Monitoring API regressions MUST run against floci-gcp rather than a Magento stack. Digest-pinned floci-gcp GCS and Floci AWS IAM OpenID Connect Provider smokes are already CI contracts. Remaining Secret Manager, Pub/Sub, Logging, and Monitoring smokes MUST NOT delay KEEP when the emulator rejects the official client; record typed unsupported instead.

#### Scenario: Integrations attach to the GCP origin

- **WHEN** New Relic, Fastly, Cloudflare, or SendGrid need live Magento evidence
- **THEN** the runner attaches them to the session-1 GCP origin and does not provision a second Magento stack

#### Scenario: AWS session skips already-proven integrations

- **WHEN** session 2 runs AWS ECS Fargate Magento after session 1 has Fastly New Relic Cloudflare and SendGrid evidence
- **THEN** those integrations are not re-executed on AWS and AWS evidence records Magento plus AWS-native remaining cells only

#### Scenario: KEEP does not invent CLI contracts

- **WHEN** dump-from-live or rollback-vs-schema has no unit-tested command
- **THEN** session 1 does not set KEEP until that command exists, rather than extending a live Magento stack while the CLI is written

#### Scenario: Integrations attach to the GCP origin

- **WHEN** New Relic, Fastly, Cloudflare, or SendGrid need live Magento evidence
- **THEN** the runner attaches them to the session-1 GCP origin and does not provision a second Magento stack

#### Scenario: AWS session skips already-proven integrations

- **WHEN** session 2 runs AWS ECS Fargate Magento after session 1 has Fastly New Relic Cloudflare and SendGrid evidence
- **THEN** those integrations are not re-executed on AWS and AWS evidence records Magento plus AWS-native remaining cells only

### Requirement: Emulators never certify provider-only behavior

Digest-pinned Floci AWS and floci-gcp suites MUST close cheaper API contracts in CI in parallel. Emulation MUST NOT mark a cell certified for GKE Autopilot, Memorystore Valkey 9.0, Cloud Armor data plane, Google-managed TLS, Magento Cloud SQL PITR, live WAF, GitHub Actions OIDC token exchange, or regional DR. Digest-pinned Floci AWS 1.7.0 MAY close IAM Create/Get/Tag OpenID Connect Provider API contracts.

#### Scenario: floci-gcp object restore is not Autopilot evidence

- **WHEN** a GCS or Secret Manager contract passes against floci-gcp
- **THEN** evidence records an emulator API contract and leaves Autopilot Magento, Memorystore, Armor, and managed TLS claims open

## MODIFIED Requirements

### Requirement: Certification follows a test pyramid and real providers when needed

Certification MUST close cheaper layers before paid cloud: unit and static analysis, package tests, integration tests, Docker or local emulation (including digest-pinned Floci for AWS API contracts and floci-gcp for GCS, Secret Manager, Pub/Sub, Logging, Monitoring, and IAM or STS contracts where they apply, and `act` for GitHub Actions workflows), then real provider certification. Emulation MUST NOT certify behavior that only a real provider can prove (GitHub Actions OIDC token exchange, managed TLS issuance, live WAF data plane, regional DR, GKE Autopilot, Memorystore Valkey 9.0, Magento Cloud SQL PITR). Live Magento certification order MUST be GCP, then AWS. Cloudflare tests MAY use the `acourtiol.com` zone and MUST attach to the GCP origin session. AWS, Scaleway, and OVHcloud live cells MUST assume limited promotional credits of about $200: minimum viable sizes, short-lived resources, reuse where safe, explicit cleanup, and ownership tags. Scaleway Magento SKUs MUST NOT be purchased on the free plan; record Magento blocked or experimental instead.

#### Scenario: Floci is not TLS evidence

- **WHEN** a CloudFront or Cloud Armor cell has only Floci or mock graph evidence
- **THEN** the cell MUST NOT be marked certified for public HTTPS or WAF data-plane behavior

#### Scenario: OVH cell stays small

- **WHEN** an OVHcloud live certification cell is planned
- **THEN** it uses the documented essential or minimum SKU, a unique ownership marker, and destroy-on-exit, and does not provision production-size HA by default
