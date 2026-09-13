## ADDED Requirements

### Requirement: Lean core and community catalog are P2

The published CLI MUST be able to load signed, digest-pinned provider artifacts without bundling every cloud SDK. Community providers MUST install only when Cosign identity, issuer, and lockfile digest match. Magento deploy MUST stay in-process until Dial is proven. Unsigned remote execution MUST remain refused.

#### Scenario: Unsigned community plugin is refused

- **WHEN** a user would load a remote provider without Cosign identity and lock digest
- **THEN** Magelift refuses before any provider code runs
