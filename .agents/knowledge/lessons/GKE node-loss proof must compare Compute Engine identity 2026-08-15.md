---
type: lesson
title: GKE node-loss proof must compare Compute Engine identity
description: GKE Standard can reuse a Kubernetes node name and provider ID after a VM is deleted, so replacement proof must compare the underlying Compute Engine instance identity.
tags:
- gcp
- gke
- resilience
- acceptance
status: stable
generated:
  by: codex/desktop
  at: '2026-08-15'
---

In the bounded GCP HA node-loss probes `gnl815` and `gnl816`, deleting the exact
Compute Engine VM caused GKE to recreate a running VM with the same Kubernetes
node name and the same `spec.providerID`. A detector that waits for a new node
name or provider ID therefore cannot observe this replacement. The `gnl816`
probe independently observed the underlying Compute Engine numeric instance ID
change from `4949359794825582332` to `7804062623324821287` while the node name
was reused.

A node-loss acceptance cell should record the exact pre-fault instance name,
numeric ID, zone, status, and creation timestamp. After the fault, it should
require a changed numeric instance ID and a running instance in the expected
zone, then verify node readiness and application health. It may accept either a
reused node identity with a changed Compute Engine instance or a genuinely new
node identity. Requiring the old Kubernetes node object to disappear would make
the check provider-specific in the wrong way and can produce a false timeout.

This proves only VM replacement and the bounded application checks included in
the cell. It does not prove zone failure, fencing, database or queue failover,
cache loss, data integrity, disaster recovery, or provider-wide HA.

## Related

See [gcha36](../../../docs/evidence/gcp-gke-ha-standard-magento-live-gcha36-20260820.md)
for the HA cell that recorded `instanceIDChanged=verified`. Physical zone
outage remains unsupported. GKE Standard is experimental.
