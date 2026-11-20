# Architecture decisions

These records describe the design. When a release decision changes, write
a new record and link it to the decision it replaces. Record what changed,
why, and how you checked it. The evidence archive holds dated run reports.
[Architecture](../architecture.md) · [Run records](../evidence/README.md)

| Decision | Subject |
| --- | --- |
| [0001](0001-php-project-configuration.md) | PHP shop configuration |
| [0002](0002-provider-test-evidence.md) | Report evidence for each tested path |
| [0003](0003-portable-contracts-vs-topology.md) | Shared Magento contracts and provider-owned infrastructure |
| [0004](0004-ports-and-adapters.md) | Ports and adapters |
| [0005](0005-external-artifact-manifest.md) | External artifact manifest and shop-owned images |
| [0006](0006-generated-local-compose.md) | Generate Compose for local development |
| [0007](0007-github-oidc-roles.md) | Separate identities for CI and state recovery |
| [0008](0008-provider-load-path.md) | Verify provider executables before loading |
| [0009](0009-dump-seed-and-attach.md) | Explicit dump import and existing service references |
| [0010](0010-local-cloud-checks.md) | Run cloud API checks locally |
| [0011](0011-subprocess-dial-proof.md) | Run provider plans in a subprocess |
| [0012](0012-search-proof-shapes.md) | Magento search and readiness |
| [0013](0013-provider-plugin-contract.md) | Versioned provider plugin contract |
| [0014](0014-distribution-trust.md) | Verify releases before execution |
| [0015](0015-gcp-bootstrap-state-key.md) | GCP bootstrap and the remote state passphrase |
| [0016](0016-gcp-owned-node-identity.md) | Give GKE nodes a dedicated identity |
| [0017](0017-gcp-sql-zone-preference.md) | Choose the Cloud SQL primary zone at creation |
| [0018](0018-gcp-explicit-sql-edition.md) | Choose the Cloud SQL edition from the requested tier |
| [0019](0019-gcp-runtime-operation-plans.md) | Operate an existing GCP environment without a build |
| [0020](0020-gcp-native-edge-address.md) | Report the shop URL and ingress address separately |
| [0021](0021-empty-magento-database-bootstrap.md) | Bootstrap an empty Magento database |
