## ADDED Requirements

### Requirement: Catalog names default stacks and an expansion rule

The compatibility catalog MUST name a default recommended stack per supported Magento release line (PHP, Composer, web server, cache, database, search, queue). It MUST list unsupported combinations instead of implying that every permutation works. Adding a new Magento patch line, PHP version, or service major MUST require a source date from Adobe system requirements plus a verification path (local image contract, cloud adapter, or explicit unavailable). Historical certified cells MUST NOT be treated as current compatibility for a newer row.

#### Scenario: Default stack for 2.4.9

- **WHEN** a user omits optional service versions on Adobe Commerce or Magento Open Source 2.4.9
- **THEN** effective configuration resolves the documented 2.4.9 default stack and records provenance for each service

#### Scenario: New service major needs a dated row

- **WHEN** Adobe lists a new OpenSearch or Valkey major that MageLift has not verified
- **THEN** validation reports the row as unsupported or unavailable until a source-dated catalog entry and verification path exist

### Requirement: Runtime families are enumerated even when not all cells exist

The catalog MUST have a place for PHP, Composer, nginx, Varnish, Redis, Valkey, OpenSearch, MySQL, MariaDB, RabbitMQ, and ActiveMQ Artemis, and for managed provider substitutes (Aurora, ElastiCache, OpenSearch Service, Amazon MQ, Cloud SQL, Memorystore, and documented Scaleway or OVHcloud equivalents). A family without a verified cell MUST be `unsupported` or `unavailable`, not omitted.

#### Scenario: MariaDB has no verified local image

- **WHEN** a project selects MariaDB and the local catalog has no pinned image and health contract
- **THEN** local planning fails before Compose is written and names the nearest verified database family
