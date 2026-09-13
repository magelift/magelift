## ADDED Requirements

### Requirement: Lock-release errors are never discarded

When deploy holds a lock, release MUST always run. If release fails, the operator MUST see `errors.Join` (or equivalent) of the primary error and the release error. A failed Magento deploy MUST NOT hide a leftover lock.

#### Scenario: Deploy fails and unlock fails

- **WHEN** Magento cutover errors and lock release also errors
- **THEN** the CLI exit error includes both causes
