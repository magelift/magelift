## Purpose

Defines the GCP implemented Magento architecture catalog, the certified Autopilot subset, and KEEP ownership so agencies can compose GKE architectures without inheriting certified status from nearby presets.

## ADDED Requirements

### Requirement: GCP catalog is explicit YAML dimensions, not opaque presets

`docs/capability-matrix.md` plus this spec MUST list every YAML-selectable GCP architecture MageLift can plan or apply: runtime (`gke-autopilot`, `gke-standard`), Cloud SQL availability (`ZONAL`/`REGIONAL`) and backup/PITR knobs, Memorystore engine/mode/zone distribution, `openSearchMode` (`opensearch`/`disabled`), `queueMode` (`database`/`rabbitmq`) and replica counts, Standard node/spot knobs, Autopilot requests, Armor, and web replica/consumer counts. Presets MAY fill omitted fields. Each cell MUST carry Adobe, MageLift-implemented, and certified-or-experimental-or-unavailable status. An omitted combination MUST be typed unavailable or fail closed.

#### Scenario: An agency selects Autopilot with regional Cloud SQL and RabbitMQ

- **WHEN** YAML sets `runtime: gke-autopilot`, `cloudSqlAvailability: REGIONAL`, and `queueMode: rabbitmq`
- **THEN** the plan accepts or fail-closes with a typed reason and does not require the operator to pick the `standard` preset to express those dimensions

### Requirement: Certified GCP architectures are Autopilot evidenced runtime cells

Only GKE Autopilot cells with `docs/capability-matrix.md` plus `docs/evidence/` for that exact Magento release and service tuple MAY be certified. GKE Standard, HA, Cloud Armor Magento `requestBodiesToExclude`, Cloud SQL brownfield attach, and Pub/Sub Magento modules MUST remain experimental, withheld, or named gaps. They MUST NOT inherit Autopilot certified.

#### Scenario: Standard Magento evidence does not certify Autopilot HA

- **WHEN** a GKE Standard 2.4.9 cell has bounded application evidence
- **THEN** the matrix keeps Standard experimental and does not mark Autopilot HA certified

### Requirement: GCP KEEP is independent of other providers

GCP live certification MUST use its own KEEP prefix, workdir, artifact digest, and destroy-on-close. Packed-campaign task 6.2 is owned by this capability until archived. AWS, OVH, or Scaleway cell status MUST NOT block recording GCP status. Vendors (Cloudflare/Fastly/New Relic/SendGrid) attach to a GCP origin only when this spec or `external-service-certification` allows it.

#### Scenario: GCP KEEP proceeds while AWS Aurora is in progress

- **WHEN** GCP Autopilot KEEP has passed and AWS KEEP is still mutating Aurora
- **THEN** `certification-gcp` records the GCP KEEP evidence independently and does not wait for AWS teardown
