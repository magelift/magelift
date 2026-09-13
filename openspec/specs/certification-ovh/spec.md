## Purpose

Defines the OVHcloud implemented Magento architecture catalog and the experimental live bar so EU agencies can see which MKS/MySQL/Valkey shapes MageLift will plan, without treating a unit or one preview as certification.

## Requirements

### Requirement: OVH catalog lists every implemented architecture family

`docs/capability-matrix.md` plus this spec MUST list every YAML-selectable OVH architecture MageLift can plan or apply: MKS plan (`free`/`standard`), region/zones, node flavor/count, managed MySQL plan/version/node count/backup, managed Valkey plan/version/node count, floating IPs, private-network default routing, and Logs Data Platform via opaque `nativeReference`. Each cell MUST carry Adobe, MageLift-implemented, and certified-or-experimental-or-unavailable status. Bootstrap and application Secrets remain `ErrNotSupported` until implemented. An omitted combination MUST be typed unavailable or fail closed.

#### Scenario: An agency selects MKS standard with MySQL 8.4 business and Valkey 8.1

- **WHEN** YAML sets `mksPlan: standard`, `databaseVersion: "8.4"`, `databasePlan: business`, and `valkeyVersion: "8.1"`
- **THEN** the plan accepts or fail-closes with a typed reason and does not mark the cell certified

### Requirement: Certified OVH subset stays empty until matrix plus evidence agree

OVH MKS MUST remain experimental. The certified subset is empty until `docs/capability-matrix.md` plus `docs/evidence/` record live Magento for that exact tuple. Unit tests, Floci, and account-free cost classification MUST NOT certify Magento.

#### Scenario: Shared kube.Steps passing does not certify MKS

- **WHEN** OVH uses shared Kubernetes Observe/Steps offline tests
- **THEN** the matrix keeps `ovh`/`mks` experimental and publishes no certified OVH row

### Requirement: OVH live bar is one preview with no KEEP

OVH live Magento MUST be at most one MKS preview when credits allow. KEEP is forbidden. Packed-campaign task 6.4 OVH is owned by this capability until archived. Private-network routing MUST use the documented DHCP gateway path; custom gateway routing MUST NOT be inferred.

#### Scenario: Campaign does not retain an OVH shop

- **WHEN** an OVH MKS Magento preview finishes pass or fail
- **THEN** the runner destroys the stack and does not set an OVH KEEP flag
