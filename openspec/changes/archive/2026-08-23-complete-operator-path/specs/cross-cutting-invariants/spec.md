## ADDED Requirements

### Requirement: RabbitMQ node count is a cold boundary

MageLift MUST refuse an in-place change between a single-node Magento broker and a quorum or multi-node HA broker. That change MUST require a new cold baseline, data-safe cutover, or an explicit unsupported result.

#### Scenario: Preview single-node cannot become production quorum in place

- **WHEN** YAML changes a live environment from one RabbitMQ node to a three-node quorum
- **THEN** planning fails closed before mutate and names restore, rebuild, or a new environment as the path
