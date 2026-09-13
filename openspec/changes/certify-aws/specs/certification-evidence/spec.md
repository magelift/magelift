## ADDED Requirements

### Requirement: AWS evidence maps to certification-aws cell IDs

Each AWS live Magento (or declared infra-only) evidence file MUST identify the `certification-aws` cell: runtime, compute mode, Magento release, database, search, queue, and digest. KEEP rows MUST NOT close a certified matrix cell until destroy plus orphan assert succeed.

#### Scenario: KEEP pass is not yet certified

- **WHEN** an AWS KEEP cell reports Magento health PASS and the stack is retained
- **THEN** evidence MAY record the KEEP run and MUST NOT replace the certified matrix row until teardown and cleanup assertion complete
