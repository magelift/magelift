# Architecture decision records

ADRs record decisions that constrain public contracts, infrastructure, security, or maintenance. Until the first public release tag, this set is the **current** decision list: edit or replace a file when the product changes. After that tag, freeze the body and supersede with a new number.

When a public contract, certification tier, or topology rule changes, update the ADR and the human page in the same change. A knowledge note is not a substitute.

Each ADR states context, decision, consequences, alternatives, and provenance.

## Current decisions

- [ADR 0001](0001-yaml-only-cli.md): YAML-only CLI in the user's account. Not a host.
- [ADR 0002](0002-certified-vs-experimental.md): Certified vs experimental. Evidence is authority.
- [ADR 0003](0003-portable-contracts-vs-topology.md): Portable Magento contracts vs per-cloud topology.
- [ADR 0004](0004-ports-and-adapters.md): Ports and adapters, including day-2 `Has*` ports.
- [ADR 0005](0005-external-artifact-manifest.md): Artifact manifest lives outside the image. Cosign is Sigstore OIDC.
- [ADR 0006](0006-generated-local-compose.md): Local development is generated Compose.
- [ADR 0007](0007-github-oidc-roles.md): Separate GitHub OIDC roles for CI and state recovery.
- [ADR 0008](0008-provider-load-path.md): In-process Magento; signed go-plugin lock for published adapters.
- [ADR 0009](0009-dump-seed-and-attach.md): Dump seed after first deploy. Brownfield attach only where implemented.
- [ADR 0010](0010-live-certification.md): No paid multi-cloud CI. Packed live certification with GCP as the thorough path.
