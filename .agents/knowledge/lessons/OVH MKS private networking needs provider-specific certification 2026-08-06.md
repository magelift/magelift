---
type: lesson
title: OVH MKS private networking needs provider-specific certification
description: EU-WEST-PAR tests separated managed Gateway, floating IP, and MKS node CNI failures instead of treating one successful API request as cluster readiness.
tags: [ovh, mks, networking, kubernetes, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-06
sources:
  - id: ovh-mks-regions
    resource: https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors
    title: OVHcloud MKS regions, nodes, and storage flavors
  - id: ovh-mks-floating-ips
    resource: https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/using-floating-ips
    title: OVHcloud MKS floating IPs
  - id: ovh-gateway
    resource: https://docs.ovhcloud.com/en/guides/public-cloud/network-services/create-private-network-gateway
    title: OVHcloud private network Gateway
---

The 2026-08-06 OVH acceptance runs used EU-WEST-PAR, MKS Standard, one zone, a private network, a subnet, a managed Gateway, MySQL 8.4, and Valkey. The Magento image was immutable and the stack used `deploy --infra-only`.

The failure modes were different:

* A floating-IP-only graph failed MKS creation with `missing a gateway in nodesSubnetId`. A managed Gateway is required even when the node pool requests floating IPs.
* A managed Gateway plus floating IPs created the cluster and node, but the node had no Kubernetes address and Cilium never became ready. The runtime deployment stayed at zero ready replicas.
* A managed Gateway without floating IPs created the control plane, but the node stayed `INSTALLING` without an instance or IP for the full 600-second runtime bound.

An explicit custom `defaultVrackGateway` was also rejected in EU-WEST-PAR as unsupported. The OVH API returned `privateNetworkRoutingAsDefault: true` with an empty default gateway when the field was omitted, so the adapter must not infer that the managed Gateway makes every private-routing mode valid.

The harness destroyed both failed stacks. OVH took several minutes to delete MKS. Subnet deletion then returned 409 while ports still held IP allocations. Bounded polling succeeded after the ports were released, and independent scans found no resources with either run prefix.

Do not call OVH MKS certified from a successful Pulumi update alone. Keep region, plan, gateway, node-pool IP mode, node readiness, CNI readiness, and exact-prefix cleanup in the live acceptance contract. Test each supported region and network mode separately.
