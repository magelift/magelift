# Public release readiness

No public tag may be created until maintainers have documented:

- trademark and package-name clearance for MageLift;
- availability and ownership of GitHub, Packagist, GHCR, and documentation names;
- complete dependency license and NOTICE review;
- signed release artifacts, checksums, SBOM, provenance, and vulnerability gates;
- green required CI and acceptance checks for the release scope;
- accurate non-affiliation language and descriptive trademark usage.

The AWS OpenSearch path is an explicit release gate. ECS tasks now include a
pinned AWS SigV4 proxy sidecar and route Magento search traffic through it. Do
not call the managed search path production-ready until a real AWS matrix proves
index creation, catalog indexing, queries, reconnects, and least-privilege
behavior.

`.github/workflows/aws-integration.yml` is the spend-gated acceptance matrix for
preview, standard, and high-availability profiles. It uses GitHub OIDC and a
repository-scoped role, consumes a reviewable configuration supplied through the
integration environment, and destroys disposable resources in an exit trap. Set
`MAGELIFT_AWS_INTEGRATION_ENABLED` only after the role, environment approvals,
signed image digest, and configuration have been reviewed.

The release workflows use Release Please for Conventional Commit versioning and
GoReleaser for reproducible multi-platform archives, SHA-256 checksums, SBOMs, and
Homebrew cask publication. The release job verifies the keyless Sigstore bundle for
the checksum manifest before publishing its GitHub artifact attestation. The
container-image workflow publishes the Debian PHP and
FrankenPHP classic matrices to GHCR, attaches SBOM and SLSA provenance, and signs
and verifies each immutable digest with keyless Cosign. Configure `HOMEBREW_TAP_GITHUB_TOKEN`
only in the release environment; it is not a repository secret used by pull
requests. Configure `RELEASE_PLEASE_TOKEN` as a narrowly scoped GitHub App or
fine-grained token so release tags can trigger the artifact workflow; the default
`GITHUB_TOKEN` does not start a second workflow.

If clearance fails, rename every identifier before the first public tag.
