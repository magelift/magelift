## ADDED Requirements

### Requirement: AWS coverage is the implemented catalog plus packed KEEP

AWS certification coverage MUST enumerate required cold boundaries (Fargate, Managed Instances, EKS) and allowed warm catalog transitions on one KEEP digest. Exhaustive support means every implemented cell is listed and classified; it MUST NOT mean one live Magento shop per combination.

#### Scenario: Search modes are classified without three shops

- **WHEN** `searchMode` values `disabled`, `serverless`, and `provisioned` are all implemented
- **THEN** the catalog lists all three, live KEEP may pack compatible transitions, and only tuples with Magento evidence MAY be certified
