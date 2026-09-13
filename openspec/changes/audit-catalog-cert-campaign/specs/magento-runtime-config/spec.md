## ADDED Requirements

### Requirement: ACC and Upsun import emit Magento overlays

Config import MUST map Magento admin front name, cookie domain, CORS origins, and named consumers into `application.magento` when those values exist in the source. Unmapped keys MUST remain in the sidecar. Import MUST NOT invent Magento websites, stores, or store views.

#### Scenario: Front name survives import

- **WHEN** an ACC project sets a custom admin front name
- **THEN** generated `magelift.yaml` includes `application.magento.frontName` and AWS WAF planning uses that path
