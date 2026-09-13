## ADDED Requirements

### Requirement: Catalog rows name Adobe vs MageLift vs plugin

Every web-runtime, database, cache, search, queue, and edge choice MUST appear in the compatibility catalog with three statuses: Adobe's current table, MageLift implementation, and MageLift certification. A plugin that Adobe does not list MUST still appear when MageLift ships it, marked Magento-experimental or Adobe-unsupported. The catalog MUST NOT omit an implemented cell.

#### Scenario: Apache is listed honestly

- **WHEN** an operator reads the catalog for Magento 2.4.9
- **THEN** nginx is Adobe-supported, and both FrankenPHP classic and Apache are Adobe-unsupported MageLift plugins that require `compatibility.allowUnsupported`

### Requirement: Magento websites are not a YAML array

The catalog MUST state that Magento websites, stores, and store views live in Magento. `application.magento` MAY overlay `frontName`, cookies, CORS, consumers, and Magento-module queue transports. MageLift MUST NOT invent a `websites[]` schema that does not drive Magento config.

#### Scenario: Multi-store YAML is refused or undocumented as Magento-owned

- **WHEN** a user looks for `websites:` in MageLift YAML
- **THEN** docs and schema point at Magento scope configuration rather than a MageLift website list
