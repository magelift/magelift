## 1. Compatibility catalog

- [x] 1.1 Add typed records for Adobe Commerce 2.4.6-p15, 2.4.7-p10, 2.4.8-p5, and 2.4.9 with source metadata.
- [x] 1.2 Encode Adobe-supported, MageLift-compatible, unsupported, and unavailable statuses for database, search, cache, queue, PHP, Composer, Varnish, and nginx choices.
- [x] 1.3 Connect catalog validation to pre-mutate configuration checks and preserve `allowUnsupported` as an explicit exception.
- [x] 1.4 Add unit tests for MySQL, MariaDB, Elasticsearch, OpenSearch, Redis, Valkey, RabbitMQ, Artemis, and Varnish edge cases.
- [x] 1.5 Generate the capability matrix and release-readiness rows from catalog data where practical.
- [x] 1.6 Carry project PHP extension and Composer version requirements through the build protocol and enforce them inside the isolated builder.
- [x] 1.7 Import ACC and Upsun runtime extension and Composer declarations into the portable build contract.

## 2. Shared acceptance contract

- [x] 2.1 Define stable cell IDs, dimensions, evidence statuses, and provenance fields.
- [x] 2.2 Convert the AWS and GCP runners to append the shared evidence shape.
- [x] 2.3 Reject mutable image tags and hand-authored PASS rows in the evidence verifier.
- [x] 2.4 Add edition-aware artifact and composer credential checks.

## 3. Provider acceptance

- [x] 3.1 Add live acceptance configuration and cleanup adapters for OVHcloud MKS.
- [x] 3.2 Add live acceptance configuration and cleanup adapters for Scaleway Kapsule.
- [x] 3.3 Add AWS ECS and EKS cell descriptors without claiming EKS certification before live evidence passes.
- [x] 3.4 Add GCP GKE cell descriptors for the catalog-required service combinations.
- [x] 3.5 Add provider-specific cost and duration capture.

## 4. Teardown and release gate

- [x] 4.1 Add unique prefix and ownership marker generation for every provider.
- [x] 4.2 Add bounded orphan polling for AWS, GCP, OVHcloud, and Scaleway.
- [x] 4.3 Add interruption and resume tests for every live harness. Offline dry-run interruption/resume and checkpoint-shape tests cover AWS/ECS, AWS/EKS, GCP/GKE, OVH/MKS, and Scaleway/Kapsule harness paths; live provider reruns remain evidence-gated by credentials and disposable resources.
- [x] 4.4 Make the expanded RC1 gate fail on required cells without PASS and cleanup proof.
- [x] 4.5 Update capability and release documents only from generated evidence. `make certification-docs-check` passes against the source-dated catalog and sealed-evidence loader; the generated capability report remains explicitly evidence-gated when no sealed live bundle is present.
- [x] 4.6 Remove temporary acceptance credentials and verify account scans after the final run. The 2026-08-09 direct owning-service audit found no marked acceptance credentials/resources; an unowned GCP secret was preserved and prior AWS acceptance secrets remain only as documented deletion tombstones.
