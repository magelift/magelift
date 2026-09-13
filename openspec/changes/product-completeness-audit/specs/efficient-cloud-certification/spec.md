## ADDED Requirements

### Requirement: Certification follows a test pyramid and real providers when needed

Certification MUST close cheaper layers before paid cloud: unit and static analysis, package tests, integration tests, Docker or local emulation (including Floci for AWS and GCP API contracts where they apply, and `act` for GitHub Actions workflows), then real provider certification. Emulation MUST NOT certify behavior that only a real provider can prove (IAM, managed TLS issuance, live WAF data plane, regional DR). Live certification order MUST be GCP, then AWS, then Scaleway, then OVHcloud. Cloudflare tests MAY use the `acourtiol.com` zone. AWS, Scaleway, and OVHcloud live cells MUST assume limited promotional credits of about $200: minimum viable sizes, short-lived resources, reuse where safe, explicit cleanup, and ownership tags.

#### Scenario: Floci is not TLS evidence

- **WHEN** a CloudFront or Cloud Armor cell has only Floci or mock graph evidence
- **THEN** the cell MUST NOT be marked certified for public HTTPS or WAF data-plane behavior

#### Scenario: OVH cell stays small

- **WHEN** an OVHcloud live certification cell is planned
- **THEN** it uses the documented essential or minimum SKU, a unique ownership marker, and destroy-on-exit, and does not provision production-size HA by default
