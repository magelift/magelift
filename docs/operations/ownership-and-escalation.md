# Operational ownership and escalation

This document describes the ownership boundary for recovery, telemetry, and edge operations. Provider SDKs and community extensions implement the native calls; the MageLift core owns validation, fingerprints, idempotency, checkpoints, evidence requirements, and cleanup gates.

## Automation-owned actions

Automation may perform these actions only inside an admitted ownership scope and with a bounded context:

- validate provider credentials as references and refuse plaintext values;
- build a provider-neutral architecture, observability, edge, and recovery plan;
- create and poll provider operations using stable idempotency keys;
- create backups, restore into an explicitly selected destination, and write operation identities to the checkpoint;
- verify scrubbed known-content manifests, checksums, counts, permissions, health, and measured durations;
- configure provider-native telemetry and external OTLP destinations through their adapters;
- apply only health-gated edge routes, purge owned content, and verify post-purge behavior;
- delete resources carrying the exact ownership marker and reconcile delayed provider tombstones.

Automation must stop and report `BLOCKED` when a capability is unavailable, a credential or quota admission check fails, an operation exceeds its timeout budget, a provider returns an unknown state, or an inventory contains an unmarked resource.

## Operator-owned actions

An operator must explicitly approve or perform actions that can create a second writer or affect a live customer route:

- failover to another region or provider;
- fencing the previous writer and proving that stale origins cannot accept writes;
- restore-in-place over an existing durable service;
- failback after the primary region or origin has recovered;
- rotating or revoking credentials when the provider cannot expose a scoped, auditable API operation;
- resolving protected resources, delayed deletion, account-level tombstones, or resources without an exact MageLift ownership marker.

The operator records the action, approval identity, provider operation ID, destination, measured RPO/RTO, and the resulting evidence record. Secret values, access tokens, certificate private keys, and database passwords never belong in the record.

## Escalation rules

1. A failed or ambiguous provider operation is retried only by the provider adapter within its declared budget. The core checkpoint remains authoritative for completed stages.
2. A data-integrity mismatch escalates to the data owner and blocks failover or certification; it is not converted into a rebuild success.
3. A fencing or route-health check that fails escalates to the on-call operator and blocks traffic mutation.
4. A cleanup mismatch escalates to the resource owner. Unmarked resources are preserved, and delayed tombstones are reported separately from live resources.
5. A provider or integration without an adapter is recorded as `unavailable` or `unsupported`; it is never represented as a successful no-op.

## Provider and extension boundary

AWS, GCP, Scaleway, OVHcloud, Fastly, New Relic, and community implementations receive the same semantic intent and ownership marker. Their SDK/API schemas remain inside the implementation package. Adding a provider therefore adds an adapter and capability records; it does not add a second core recovery, evidence, or scheduling model.
