## ADDED Requirements

### Requirement: Operators can request Magento-safe purge

Production native or Fastly edge profiles that cache Magento MUST support a Magelift purge or invalidate action after deploy and on operator request. Unmodified vendor CRS MUST NOT be certified as Magento-protected.

#### Scenario: Post-deploy purge runs

- **WHEN** a Magento deploy completes on a profile with Magento-facing CDN cache
- **THEN** Magelift issues the documented purge and does not require a vendor console
