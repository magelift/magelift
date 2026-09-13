## 1. Runtime contract

- [x] 1.1 Audit the existing configuration, build protocol, compatibility catalog, and evidence fields against the `runtime-requirements` spec.
- [x] 1.2 Add table-driven validation tests for PHP extensions, Composer versions, release lines, editions, and secret-reference requirements.
- [x] 1.3 Make ACC and Upsun import tests preserve supported runtime declarations and report unmapped hooks without executing them.
- [x] 1.4 Seal the resolved runtime contract into generated plan and certification evidence without secret values.

## 2. Warm certification sessions

- [x] 2.1 Consolidate the AWS and GCP checkpoint fingerprints around the fields required by `warm-certification-sessions`.
- [x] 2.2 Add tests for compatible service transitions, stale fingerprints, schema-changing boundaries, and interrupted resume.
- [x] 2.3 Make every warm cell record baseline versus reused status and per-cell timing in generated evidence.
- [x] 2.4 Verify one final dependency-aware teardown and direct inventory assertion for each provider harness.

## 3. External services

- [x] 3.1 Review Fastly, Cloudflare, CloudFront, Cloud Armor, and observability declarations against the `external-service-certification` contract.
- [x] 3.2 Finish a registered Fastly adapter using secret references, current domain lifecycle operations, purge policy, and exact cleanup markers.
- [x] 3.3 Define the first third-party observability adapter contract for logs, metrics, traces, credentials, and teardown without adding vendor fields to core YAML.
- [x] 3.4 Add import and validation tests for ACC and Upsun edge and observability intent, including unmapped-field reporting.

## 4. Matrix closure

- [x] 4.1 Regenerate the release/service matrix from the dated Adobe requirements source for 2.4.6-p15 through 2.4.9.
- [x] 4.2 Run warm certification groups for MySQL, MariaDB, OpenSearch, Elasticsearch where allowed, Valkey, Redis where allowed, RabbitMQ, Artemis, database messaging, Varnish, and no-Varnish choices. Existing live warm groups are recorded with their exact scope; MariaDB, Elasticsearch, Redis, and unavailable Varnish groups remain explicit not-run, unsupported, or unavailable cells.
- [x] 4.3 Add cold baseline cells for every new provider, Kubernetes topology, database engine, and major service version. See `scripts/acceptance/cold-baselines-2026-08-08.tsv` and `docs/evidence/expanded-rc1-matrix-2026-08-08.md`.
- [x] 4.4 Run the remaining AWS EKS, OVH MKS, and Scaleway Kapsule cells only where the provider capability intersection is available. AWS EKS runtime cells are evidenced; OVH is blocked before Magento health and Scaleway Redis is Adobe-unsupported, so no paid apply was attempted for either.
- [x] 4.5 Keep every unrun, blocked, unavailable, or experimental cell visible in the release gate and evidence index.
- [x] 4.6 Define a standalone certification-matrix contract for cell dimensions, provider intersections, evidence thresholds, warm boundaries, cost reporting, and cleanup truth.

## 5. Release and operations

- [x] 5.1 Add a single operator command that prints the planned warm groups, estimated cost, expected teardown scope, and required credentials before apply.
- [x] 5.2 Add provider cleanup reports that distinguish live resources, delayed tombstones, protected user resources, and provider metadata that cannot be hard-deleted.
- [x] 5.3 Run the serial Go test, shell acceptance-shape tests, docs validation, and knowledge-bundle validation with the one-build-at-a-time memory limits.
- [x] 5.4 Update release-readiness, capability, certification evidence, and handoff documentation from generated results.
