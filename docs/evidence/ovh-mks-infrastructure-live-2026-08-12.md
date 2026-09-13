# OVHcloud MKS infrastructure live acceptance — 2026-08-12

Status: **PASS for the bounded infrastructure-only cell**. This record does
not certify Magento runtime health, multi-zone HA, backup/restore, regional
DR, observability collection, or edge behavior.

## Cell

- Provider project: redacted
- Region: `EU-WEST-PAR`
- MKS: current `standard` plan, one worker in one selected zone
- Managed database: MySQL `8.4`
- Managed cache: Valkey `8.1`, one-node discovery profile
- Network: one disposable private network with a configured `/16`; the
  provider-created regional subnet was `/24` with DHCP enabled
- Artifact: public immutable NGINX digest, used only as a pullable
  infrastructure smoke image; it is not Magento-compatible
- Harness: `scripts/ovh-acceptance-local.sh`, `deploy --infra-only`
- Ownership prefix: `ovh812`
- Catalog spec: `certification-ovh`
- Cell: region `EU-WEST-PAR`, MKS `standard`, MySQL `8.4`, Valkey `8.1`, digest is NGINX infra-only (not Magento)

## Result

PASS:

1. Read-only admission authenticated through the selected OVHcloud profile,
   resolved the current Public Cloud project, and accepted the target region,
   MKS plan, zone, MySQL, Valkey, and worker catalog before mutation.
2. The run created 23 resources: the private network and subnet, gateway,
   managed MySQL and Valkey services, MKS cluster and node pool, Kubernetes
   provider and secrets, Deployments, and a LoadBalancer Service.
3. Provisioning completed in `8m53s`. The infrastructure-only stage reached
   its expected outputs and deliberately did not run Magento migration or
   runtime health because the immutable artifact is NGINX.
4. Pulumi removed the managed cluster, node pool, databases, gateway, and
   normal dependent resources. The OVH API temporarily rejected subnet
   deletion with HTTP 409 because ports still held IP allocations. The
   ownership-scoped cleanup loop retried the subnet/network after provider
   convergence and completed without broad deletion.
5. The harness returned `ovh assert_clean ok`. Fresh direct inventories after
   the run returned empty results for MKS clusters, managed databases, load
   balancers, and the exact acceptance private network.

## Implementation corrections discovered by the live cell

- The current OVH project response uses `project_id`; the provider identity
  and capability decoders now accept that field while retaining compatibility
  with the provider's other documented identity fields.
- OVH's documented private-subnet DHCP mode requires an empty
  `default_vrack_gateway` together with
  `private_network_routing_as_default=true`. The runtime no longer guesses a
  custom gateway from the first usable subnet address.
- Subnet deletion must tolerate the provider's asynchronous port/IP release.
  Cleanup therefore keeps retrying the exact owned subnet/network and reports
  the owning-service inventory result instead of treating the first 409 as a
  final failure.

## Explicit non-claims

- `--infra-only` skipped Magento migration, cutover, and runtime health.
- One worker and one selected zone do not prove MKS multi-zone HA, node
  failure recovery, or cross-zone traffic behavior.
- No backup, restore, known-content integrity, regional DR, fencing,
  failback, collector delivery, audit-stream delivery, or edge traffic test
  was run.
- This result closes only the deterministic fake-client plus disposable MKS
  infrastructure lifecycle evidence for this cell; it does not promote OVH
  to production-certified status.
