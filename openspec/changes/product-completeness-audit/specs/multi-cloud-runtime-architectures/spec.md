## ADDED Requirements

### Requirement: Architecture profiles cover Magento production building blocks

Each first-party architecture profile MUST declare how it supplies, substitutes, or marks unavailable: networking, DNS, TLS, load balancing, compute, autoscaling, object storage, database, cache, search, queue, persistent or shared storage, CDN, WAF, secrets, IAM, monitoring, logging, backups, restore, resource tags, and lifecycle policies. Managed provider services MUST be used when the catalog selects them (for example AWS Aurora, S3, Amazon MQ, ElastiCache, OpenSearch Service, SES; GCP Cloud SQL, Cloud Storage, Memorystore, Pub/Sub, Cloud Armor; documented Scaleway and OVHcloud equivalents). A missing building block MUST be typed unavailable, not silently omitted from the profile.

#### Scenario: Profile lists search as unavailable

- **WHEN** a Scaleway or OVHcloud profile has no source-dated managed search product in MageLift
- **THEN** planning reports search unavailable and does not invent a substitute product name
