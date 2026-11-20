# ADR 0005. Write the artifact manifest beside the image

Accepted on 2026-08-22.

The manifest names the finished OCI image by digest. Writing it into the
image would change that digest, so MageLift keeps it as a separate artifact.

## Build and promotion

The PHP `prepare` step records the source revision, versions, enabled modules,
checksums and static assets. BuildKit creates the image. PHP `finalize` combines
the prepared metadata with the returned digest, without rereading the source.
The manifest and image labels keep the build traceable. Keep secret values in
the shop's secret store.

The shop's Docker host or CI runner builds the image. The CLI bundles the
nginx/PHP-FPM recipes and pins upstream bases. `build.php` selects the same PHP
branch for the builder and runtime. Local builds load the result into Docker.
Pushed builds use the shop's OCI registry. For bundled builds, the optional
`buildRecipe` entry records the recipe checksum, platform and pinned bases.
An explicit base-image override records the selected image references.

MageLift signs the pushed digest with Cosign and the operator's or CI job's
Sigstore OIDC identity. Promotion and rollback use the recorded digest.
When copying the image to another registry, copy the signature or sign the
destination separately. [Application builds](../builds.md) · [Promote a release](../operations.md)
