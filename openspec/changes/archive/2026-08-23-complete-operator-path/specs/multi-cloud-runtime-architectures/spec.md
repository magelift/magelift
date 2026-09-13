## ADDED Requirements

### Requirement: Cache and sessions are isolatable

Durable Magento profiles MUST be able to isolate object cache from PHP sessions on Valkey or Redis. Preview MAY share one cache for cost. Local Docker MUST follow the same isolation when the selected environment is durable.

#### Scenario: Production isolates sessions

- **WHEN** a standard or high-availability Magento profile is planned
- **THEN** cache and session endpoints are distinct and Magento configuration does not point sessions at an allkeys-lru cache

### Requirement: Dense production profiles are explicit and experimental until evidenced

Catalog profiles MAY describe ECS Managed Instances, Aurora, Magento OpenSearch data-plane, RabbitMQ quorum, and Magento-safe CloudFront. Those cells MUST remain experimental or unavailable until `docs/capability-matrix.md` plus evidence say otherwise. Experimental-but-implemented cells MUST warn and MUST NOT block plan or apply. Unavailable cells MUST fail closed. They MUST NOT be the P0 success bar for Magento PHP SMEs and MUST NOT be claimed certified.

#### Scenario: Uncertified Managed Instances stay experimental

- **WHEN** YAML selects ECS Managed Instances without a certified cell
- **THEN** planning warns that the cell is MageLift-experimental, proceeds to mutate, does not require `compatibility.allowUnsupported`, and does not claim production certification

### Requirement: Magento search is index, query, and auth

A search cell MUST NOT be certified as Magento search solely because a search product exists. Certification MUST include Magento reindex or query evidence and the documented authentication path.

#### Scenario: OpenSearch box is not Magento search

- **WHEN** infrastructure created an OpenSearch domain but Magento never indexed or queried it
- **THEN** the cell MUST NOT be marked certified for Magento search
