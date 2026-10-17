# Application builds

`magelift build` prepares Magento in the pinned builder image and creates an application image from the result. The source repository must be clean. MageLift writes the final artifact manifest outside the repository.

For a local build, run:

```sh
magelift build
```

Run from the shop repository, with `magelift.yaml` and `composer.lock` committed.
Existing `app/etc/config.php` is an immutable input; disabled modules stay disabled.
The default community recipe uses Magento Open Source 2.4.9, PHP 8.5 and
nginx-fpm on `linux/amd64`, including on an ARM workstation. The CLI ships the
existing Docker and PHP build recipes with pinned upstream image digests.
It builds the builder and runtime images on your Docker engine, then builds
the application. BuildKit can reuse its cache. No MageLift source checkout or
MageLift image registry is required. Native ARM runtime qualification remains
NOT_RUN.

BuildKit temporary bind mounts are created below the user cache so Docker Desktop,
Colima, and other `/Users`-shared Docker engines can access them. The active Docker
Buildx builder is used by default; set `BUILDX_BUILDER` when a named builder such as
`colima` is required.

## Push a release candidate

Run this command in your CI/CD after authenticating Docker to your registry.
MageLift builds and publishes the builder, runtime and application images in
the repository you select. Builder/runtime tags include the shipped recipe
fingerprint; the application consumes their resulting registry digests.
No image is published to a MageLift-owned namespace.

```sh
magelift build --push --image ghcr.io/example/shop:candidate
```

The same repository holds all three images, using separate tags; for ECR,
create that repository and grant CI push/pull permissions. GHCR, ECR,
Artifact Registry and other OCI registries use the same `--image` option.
For example, `123456789012.dkr.ecr.us-east-1.amazonaws.com/shop:candidate`
selects an ECR repository. MageLift uses Docker's configured registry login.
Registry retention policies must preserve the referenced base digests and
attestations. Interrupted builds may leave published base images; MageLift
does not delete registry artifacts automatically.

A local `magelift build` loads images into Docker without publishing them.
Pushed builds also default to `linux/amd64`. Advanced `--builder-image` and
`--runtime-image` overrides must be supplied together. Pushed overrides require
registry digests. Explicit ARM or multiple-platform builds require those
advanced images and remain outside the community alpha qualification.

The Git `origin` must be an HTTPS URL. Repositories with an SSH origin can pass the public source URL explicitly:

```sh
magelift build --push \
  --source-url https://github.com/example/shop.git \
  --image ghcr.io/example/shop:candidate
```

The CLI adds the full inspected commit checksum to the provenance URL. It rejects URL credentials and unrelated query parameters so they cannot enter image labels or attestations.

BuildKit publishes an SBOM and SLSA provenance with the image. MageLift then signs the pushed digest. `magelift build --push` and `magelift sign` use the current `gcloud` or CI login; GCP impersonates the Magento CI service account from `magelift bootstrap` so operators never pass Cosign flags. Identity tokens are passed to Cosign only as a file path (never a JWT on argv). CR/LF is stripped first. Advanced automation may still set `MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE`, `MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV`, or `--identity-token-file`. With no Magelift or `gcloud` token source, Cosign uses ambient OIDC (GitHub Actions or a one-time browser confirmation). Artifact Registry, ECR, and Binary Authorization signatures are not substitutes. The release workflow pins Cosign 3.1.3 through the verified installer action.

Composer credentials are mounted as a private file during preparation. They are not passed to BuildKit, copied into the image, or written to the artifact manifest.

Before BuildKit creates the application image, MageLift compares the isolated
builder's reported PHP branch, Composer version, and loaded extensions with the
requested build contract. A builder response that does not satisfy the request
stops the build before image creation; the `opcache` request is satisfied by the
PHP runtime's `Zend OPcache` module name, which is normalized to the stable
`zend_opcache` artifact identifier.

## Traceability

The existing external manifest records the source revision, locked input
hashes, PHP and Composer versions, modules, static-content matrix and final
image digest. Its MageLift version comes from the CLI; development builds use
`0.0.0-dev`. The build-package version remains independent.

The image carries `io.magelift.builder.image` and `io.magelift.runtime.image`
labels with the resolved base identities. The final digest links those labels
to the external manifest. Repeat builds may have different OCI digests while
retaining equivalent declared inputs and application content.

## Preparation hooks

Hooks extend the validated preparation DAG with an argument vector. The executable
is limited to Composer or `bin/magento`; shell strings are not accepted.

```yaml
build:
  hooks:
    build.prepare:
      phase: build
      relationship: before
      target: build
      command:
        executable: composer
        arguments: [run-script, prepare]
```

Hooks are ordered by their target and dependency edges. Retries require
`idempotent: true`, and each hook has a bounded timeout. YAML hooks are limited to
the validate, build, and package phases; deployment-time hooks are provider
extension concerns until the deployment protocol exposes the same typed contract.
