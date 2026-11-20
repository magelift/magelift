# ADR 0019. Operate an existing GCP environment without a build

Accepted on 2026-10-18.

A developer needs to read outputs, logs or health from another workstation
even when the image was built in CI.

The CLI uses the optional `runtimeObserve` flag on a Plan request for
operations that read existing stack outputs. GCP allows an absent digest on
this path. It still rejects a malformed digest and checks provider, backend,
topology and preview ownership. The flag is a planning option. An ordinary
preview or apply requires a complete deployment plan with a valid artifact.

Teardown can run without an image digest. The provider previews the
destruction against saved Pulumi state and disables execution of the
application program. The teardown scope selects this path. Project ownership,
environment protection and preview expiry checks still apply.

Local tests cover flag forwarding, malformed digests, rejection of observation
plans by deploy operations and teardown preview. [Operate a shop](../operations.md)
