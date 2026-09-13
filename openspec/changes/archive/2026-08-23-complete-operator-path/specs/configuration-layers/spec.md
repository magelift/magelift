## ADDED Requirements

### Requirement: Architecture examples are copyable YAML

MageLift MUST ship example `magelift.yaml` documents under `examples/` for preview-cheap, production-like, integrated storefront, and headless Magento. Examples MUST resolve to existing preset and catalog fields with provenance. They MUST NOT introduce a second catalog vocabulary. `magelift init` MUST be able to start from a named example.

#### Scenario: Agency developer copies an integrated storefront example

- **WHEN** the user initializes from the documented integrated storefront example
- **THEN** effective configuration uses nginx-fpm, an environment preset, and Magento storefront URL fields, and every value has provenance
