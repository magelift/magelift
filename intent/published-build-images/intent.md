---
status: superseded
slug: published-build-images
---

# Superseded: published Magento build foundation

The original central-image objective is replaced by ML-03 in maintainer plan
revision 1.1. Preserve the existing recipes and useful checks. Intermediate
builder/runtime images stay on the runner; publish deployable images and an
optional user-owned cache. A real minimal user CI workflow belongs to milestone A.

## Problem

The maintainer requires every image to be built in the user's CI/CD and
published to their chosen OCI repository. The original proposal for a public
MageLift base-image catalog is superseded. Requiring a framework checkout or
manual base-image assembly would still make onboarding unnecessarily complex.

## Evidence

[GCP onboarding](../../docs/onboarding.md) currently contains Docker Buildx
commands for MageLift builder and runtime images before the shop build.

[ArtifactManifest](../../build/src/Artifact/ArtifactManifest.php) already
records the Magento, PHP, Composer, module, checksum, static-content, capability,
and image facts required by the application boundary.
[ImmutableArtifactContract](../../sdk/artifact.go) already binds image,
manifest, input, provenance, and signature identities.

The clean-room production lessons
[build without runtime env.php](../../.agents/knowledge/lessons/Magento%20image%20builds%20must%20not%20see%20a%20runtime%20env.php.md)
and
[deploy before traffic moves](../../.agents/knowledge/lessons/Magento%20deploy%20writes%20env.php%20before%20traffic%20moves.md)
establish the required lifecycle split.

## Proposed outcome

The CLI ships the existing recipes and PHP build package. A shop's CI builds
the builder, runtime and application images and publishes them to the same
user-selected OCI repository. Bases are consumed by their resulting digests.
Local builds load images without publishing. The existing external manifest
and traceability remain. Builds need no runtime database or plaintext secrets.

## Affected users and systems

Magento developers and CI; build package; build pipeline; container registry;
release workflow; image scanning and provenance; GCP onboarding; later AWS
onboarding.

## Constraints

- Reuse ArtifactManifest, ImmutableArtifactContract, and BuildArtifact.
- Do not introduce a second application manifest.
- Ship one tested recipe; do not expand the compatibility matrix.
- Base-image and application-image digests are immutable.
- Shop source and Composer credentials remain user-owned.
- Composer credentials use the existing secret mount and never enter image
  layers, manifest, logs, or evidence.
- Runtime env.php and cloud endpoints are materialized only during deployment.
- Advanced base-image overrides must narrow or invalidate certification claims.

## Out of scope

- Replacing the Magento-specific builder with Cloud Native Buildpacks.
- Building the user's application inside a hosted MageLift service.
- GCP provisioning or the public alpha tag.
- New Magento/PHP compatibility rows.
- Provider-specific application images.

## Maintainer decision

No MageLift-owned image publication or required registry. Use the user's
Docker authentication and OCI repository. Registry retention is user-owned;
MageLift does not automatically delete images after an interrupted build.
