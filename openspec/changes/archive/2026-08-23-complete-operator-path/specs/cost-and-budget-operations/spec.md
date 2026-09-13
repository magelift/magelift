## ADDED Requirements

### Requirement: Cost is visible before apply per environment

`magelift cost` MUST estimate or classify spend for the selected environment and account binding before a Magento PHP operator applies a catalog change. Preview MUST NOT display production account budget limits as if they were preview limits.

#### Scenario: Agency checks cost for one client

- **WHEN** the operator runs `magelift cost --env staging` for a client project
- **THEN** the report is scoped to that environment's catalog and does not print another client's account budget
