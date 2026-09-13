## ADDED Requirements

### Requirement: Adobe-unsupported combinations fail closed unless allowUnsupported

A combination Adobe system requirements mark unsupported MUST fail configuration validation before preview, local Compose mutation, or cloud mutation. `compatibility.allowUnsupported` MAY proceed past that Adobe gate only. It MUST NOT recertify the cell, MUST NOT hide the Adobe-unsupported status in provenance, and MUST NOT apply to MageLift-experimental or provider-unavailable cells. Validation errors MUST name which authority rejected the combination: Adobe, MageLift, or the selected provider.

#### Scenario: Adobe-unsupported MySQL fails closed

- **WHEN** a 2.4.6-p15 project selects MySQL and `compatibility.allowUnsupported` is unset
- **THEN** validation exits before plan or mutate, names Adobe as the rejecting authority, and names the supported database family

#### Scenario: allowUnsupported is an Adobe hatch only

- **WHEN** the same Adobe-unsupported combination is selected with `compatibility.allowUnsupported: true`
- **THEN** planning may proceed with Adobe-unsupported recorded in provenance, and the cell MUST NOT be reported as `adobe-supported` or certified

### Requirement: MageLift-experimental cells warn and do not block

Selecting a MageLift-experimental or otherwise uncertified-but-implemented cell MUST print a warning that names MageLift as the authority, MUST record experimental in provenance, and MUST NOT fail validation or block plan or apply. The warning MUST NOT be silent. The cell MUST NOT be reported as certified. `compatibility.allowUnsupported` MUST NOT be required and MUST NOT recertify the cell. Provider-unavailable cells (no adapter or no SKU) MUST still fail closed.

#### Scenario: Experimental MageLift cells warn and proceed

- **WHEN** YAML selects a MageLift-experimental cell such as ECS Managed Instances
- **THEN** `config validate`, plan, and mutate succeed, a warning names MageLift and experimental, provenance records experimental, and the cell is not claimed certified

#### Scenario: Unavailable stays blocking

- **WHEN** YAML selects a provider-unavailable cell with no adapter or no SKU
- **THEN** validation fails before mutate, names the provider as the rejecting authority, and does not treat the cell as experimental-warn

### Requirement: nginx is the only catalog web family

Catalog rows for Magento 2.4.x web runtimes MUST list nginx only. Apache and FrankenPHP MUST be absent from the catalog, schema, and recipes.

#### Scenario: nginx is the only web family

- **WHEN** a catalog row is listed for Magento 2.4.x web runtimes
- **THEN** the Adobe-aligned web family is nginx, and Apache and FrankenPHP are absent from the row

### Requirement: SQS and Pub/Sub are never Adobe-supported Magento brokers

Magento core message queue remains database or AMQP. SQS and Pub/Sub MUST NOT appear as `adobe-supported` catalog brokers. They MAY be provisioned only as an optional Magento-module transport when a named Composer Magento package is locked. Without that package, planning MUST fail closed. Core Magento consumers MUST stay on db or AMQP.

#### Scenario: SQS without a Magento module is refused

- **WHEN** YAML requests SQS or Pub/Sub and the project lock does not contain the declared Magento module
- **THEN** validation fails before provision and names the module contract
