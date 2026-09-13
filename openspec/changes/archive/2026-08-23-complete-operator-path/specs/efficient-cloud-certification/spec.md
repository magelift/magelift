## MODIFIED Requirements

### Requirement: Certification follows a test pyramid and real providers when needed

Certification MUST close cheaper layers before paid cloud: unit and static analysis, package tests, integration tests, Docker or local emulation (including digest-pinned Floci for AWS API contracts and floci-gcp for GCS, Secret Manager, Pub/Sub, Logging, Monitoring, and IAM or STS contracts where they apply, and `act` for GitHub Actions workflows), then real provider certification. Emulation MUST NOT certify behavior that only a real provider can prove (GitHub Actions OIDC token exchange, managed TLS issuance, live WAF data plane, regional DR, GKE Autopilot, Memorystore Valkey 9.0, Magento Cloud SQL PITR). Live Magento certification order MUST be GCP first. Cloudflare, Fastly, New Relic, and SendGrid tests MUST attach to the GCP origin session when the vendor is the claim. AWS, Scaleway, OVHcloud, Cloudflare, Fastly, New Relic, and SendGrid live cells MUST assume thin paid credits: minimum viable sizes, short-lived Magento, reuse of slow GCP KEEP resources where safe, explicit cleanup, and ownership tags. Scaleway Magento SKUs MUST NOT be purchased on a plan that cannot afford them; record Magento blocked or experimental instead.

#### Scenario: Floci is not TLS evidence

- **WHEN** a CloudFront or Cloud Armor cell has only Floci or mock graph evidence
- **THEN** the cell MUST NOT be marked certified for public HTTPS or WAF data-plane behavior

#### Scenario: OVH cell stays small

- **WHEN** an OVHcloud live certification cell is planned
- **THEN** it uses the documented essential or minimum SKU, a unique ownership marker, and destroy-on-exit, and does not provision production-size HA by default

#### Scenario: Paid integration attaches to GCP

- **WHEN** Fastly, New Relic, or SendGrid live evidence is required
- **THEN** the runner attaches to a packed GCP Magento session or leaves the vendor cell unproven, and does not open a second Magento origin only for that vendor
