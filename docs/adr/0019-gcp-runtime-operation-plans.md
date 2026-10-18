# ADR 0019: GCP plans for runtime operations

- Status: Accepted
- Date: 2026-10-18

## Context

User CI builds the application and passes its immutable digest to deploy. The
shop YAML does not need to contain that digest. Requiring it again to prepare
outputs, exec, logs, tunnels or health prevents diagnosis from another machine,
including after a failed first deployment. A local release journal cannot
supply that machine's missing context.

## Decision

The existing Plan request gains an optional `runtimeObserve` field, defaulting
to false. The CLI uses it for output-backed runtime operations. GCP permits an
absent image digest in this mode; an explicitly malformed digest is still
rejected. Target, envelope, topology, backend and preview ownership validation
remain in force. Account-only bootstrap planning keeps its existing meaning.

The flag is used during planning and is not stored in the opaque spec or a
second manifest. Output and runtime-observation handlers validate the resulting
scope. Apply and preview validate a complete deployment spec before constructing
a backend, so an artifact-less operation plan cannot deploy anything.

Destroy uses the existing teardown validation, which also permits an absent
image. It retains the target ownership, protected-environment and expired-preview
rules. Teardown does not require an application build.

No YAML key, RPC method, protocol version or StoredPlan shape changes. Older
providers ignore the optional request field and reject the missing digest.
Release lockfiles keep the distributed CLI and provider together. Artifact
signature, digest and publisher checks are unchanged.

## Verification

Regressions cover flag forwarding, its default value, artifact-less runtime
planning, malformed digests, rejection by apply and preview before backend
construction, and teardown without an update. Live distributed runtime operations
remain an acceptance check.
