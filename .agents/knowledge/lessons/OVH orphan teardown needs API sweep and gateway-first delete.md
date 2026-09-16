---
type: lesson
title: OVH orphan teardown needs API sweep and gateway-first delete
description: The ovhcloud CLI cannot list private networks, so sweep via the signed API reader; delete the regional gateway before subnet and network.
tags: [ovh, mks, networking, teardown, acceptance]
status: stable
generated:
  by: cursor/devbox
  at: 2026-09-15
---

# OVH orphan teardown needs API sweep and gateway-first delete

`ovhcloud cloud network private list` does not exist (only the `vrack`
subcommand does). Piping it to grep yields zero matches, which reads as clean
but is false: the rerun then fails with 400 "private network already exists
for this vlan ID". Sweep OVH networks and gateways only via the signed API
reader (`ovh_api_request` in `k8s-acceptance-local.sh`): GET
`/cloud/project/{P}/network/private` plus `/region/{R}/gateway`. Never trust
the CLI for these.

The gateway holds the subnet: a subnet delete failed 400 "One or more ports
have an IP allocation" 35+ minutes after MKS deletion because the orphan
gateway still had interfaces on it. Delete in order: regional gateway first,
then subnet, then network. A gateway delete can cascade (subnet and network
404 afterwards). Known harness gap: the orphan cleanup loop retries
subnet/network but never looks for the gateway.

Project-global gotchas: the vlan-0 slot lingers after DELETE reports success,
so the next create 400s; the gateway quota is 1 per region, so an orphan
gateway blocks new runs.

MKS flavors (verified 2026-09-15, re-check the capability list before
planning a run): B2 flavors are NOT offered (instant 404 "Flavor not found");
only Gen-3 (b3/c3/r3, 23 flavors) is. b3-8 (2 vCPU / 8 GB) is the smallest
balanced MKS flavor in PAR and MIL.

Run discipline: launch with TTL 7200 (a 3600s TTL expired mid-failure and met
the since-fixed destroy-after-expiry bug) and do not interrupt — see
[EXIT traps cannot be the cleanup authority](./EXIT%20traps%20cannot%20be%20the%20cleanup%20authority.md).
