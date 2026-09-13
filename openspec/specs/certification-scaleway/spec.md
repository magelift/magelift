## Purpose

Defines the Scaleway implemented Magento architecture catalog and the experimental live bar so agencies can see which Kapsule/RDB/Redis shapes MageLift will plan, without treating cost classification or one preview as certification.

## Requirements

### Requirement: Scaleway catalog lists every implemented architecture family

`docs/capability-matrix.md` plus this spec MUST list every YAML-selectable Scaleway architecture MageLift can plan or apply: Kapsule version/node type/count, RDB node type/HA/backup/encryption, Redis node type/version/cluster size (`cacheMode: redis`), Cockpit, and web replica/consumer counts. Each cell MUST carry Adobe, MageLift-implemented, and certified-or-experimental-or-unavailable status. Bootstrap remains `ErrNotSupported` until implemented. An omitted combination MUST be typed unavailable or fail closed.

#### Scenario: An agency selects Kapsule with RDB HA and Redis cluster size 3

- **WHEN** YAML sets `databaseHighAvailability: true` and `redisClusterSize: 3`
- **THEN** the plan accepts or fail-closes with a typed reason and does not mark the cell certified

### Requirement: Certified Scaleway subset stays empty until matrix plus evidence agree

Scaleway Kapsule MUST remain experimental. The certified subset is empty until `docs/capability-matrix.md` plus `docs/evidence/` record live Magento for that exact tuple. Account-free cost classification and shared kube.Steps MUST NOT certify Magento. Redis (not Valkey) MUST stay visible as the implemented cache family.

#### Scenario: Cost capacity support does not certify Kapsule

- **WHEN** account-free Cost reports Scaleway capacity
- **THEN** the matrix keeps `scaleway`/`kapsule` experimental and publishes no certified Scaleway row

### Requirement: Scaleway live bar is one preview inside the own-money cap with no KEEP

Scaleway live Magento MUST be at most one Kapsule preview funded from the **$50** own-money cap shared with Cloudflare, SendGrid, Fastly, and New Relic. KEEP is forbidden. Packed-campaign task 6.4 Scaleway is owned by this capability until archived. Vendors MUST attach to a GCP origin until this spec explicitly allows a Scaleway origin.

#### Scenario: Campaign does not retain a Scaleway shop

- **WHEN** a Scaleway Kapsule Magento preview finishes pass or fail
- **THEN** the runner destroys the stack, does not set a Scaleway KEEP flag, and remaining vendor spend still counts against the same $50 cap
