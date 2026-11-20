# Operational ownership and escalation

Cloud providers and community extensions make native recovery, telemetry
and edge API calls. MageLift validates requests, records their fingerprints
and checkpoints, and checks evidence and ownership before cleanup. It uses
idempotency keys so a retry refers to the same operation.

## Automation-owned actions

Automation can act on resources in the verified ownership scope within the
operation's time limit.

- Validate credential references and refuse plaintext values.
- Plan infrastructure, recovery, telemetry and edge changes using shared types.
- Start and poll provider operations with stable idempotency keys.
- Back up or restore into the selected destination. Record the operation ID in the checkpoint.
- Check scrubbed manifests, checksums, counts, permissions, health and timings.
- Configure cloud telemetry and external OTLP destinations through their adapters.
- Check route health before applying edge changes. Purge owned content and verify the result.
- Delete resources with the exact ownership marker and reconcile delayed provider tombstones.

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

1. The provider adapter handles retries within its declared budget. Use the MageLift checkpoint to see which stages completed.
2. A data-integrity mismatch blocks failover. Send the restore results to the data owner.
3. A fencing or route-health check that fails escalates to the on-call operator and blocks traffic mutation.
4. Send cleanup mismatches to the resource owner. MageLift leaves unmarked resources in place and lists delayed tombstones separately from live resources.
5. MageLift reports a missing adapter as `unavailable` or `unsupported`.

## Provider and extension boundary

Cloud and external adapters receive the operation and its ownership marker.
The implementation package contains the provider's API types. A new adapter
supplies capability records and uses the existing recovery, evidence and
scheduling contracts.
