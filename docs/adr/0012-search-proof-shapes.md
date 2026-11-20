# ADR 0012. Magento search and readiness

Accepted on 2026-09-13.

## Context

A shop needs search that Magento can reach and query after a workload restart.
A registered OpenSearch resource alone says little about that behavior.

## Decision

AWS can use a provisioned OpenSearch domain or another supported endpoint.
GKE Autopilot can run OpenSearch as a single-replica StatefulSet on persistent
storage. A shop that needs search failover must choose a redundant service.
[Service choices](../service-bindings.md)

Readiness checks use the same endpoint interpretation as Magento. A bare service
name resolves to HTTP on port 9200. An explicit port is retained. HTTP and HTTPS
URLs keep their scheme, port and path. AWS managed domains use TLS. Readiness
skips the unsigned reachability request for an endpoint that requires SigV4.
Database and search engine checks still run.

## Verification

The September 2026 Autopilot run reindexed a sample catalog, queried the
storefront, replaced the search pod and queried again with the same data.
The AWS run recorded Magento using a provisioned OpenSearch domain. Its
evidence covers the deployment and service configuration.
[Autopilot search run](../evidence/gcp-gke-autopilot-magento-search-live-mldp6-20260914.md) · [AWS provisioned OpenSearch run](../evidence/aws-ecs-fargate-packed-keep-mlaw1-20260915.md)
