# Application builds

`magelift build` prepares Magento in the pinned builder image and creates an application image from the result. The source repository must be clean. MageLift writes the final artifact manifest outside the repository.

For a local build, run:

```sh
magelift build
```

The local build uses the host platform and the pinned MageLift images already loaded in Docker.
BuildKit temporary bind mounts are created below the user cache so Docker Desktop,
Colima, and other `/Users`-shared Docker engines can access them. The active Docker
Buildx builder is used by default; set `BUILDX_BUILDER` when a named builder such as
`colima` is required.

## Push a release candidate

A pushed build requires registry digests for both MageLift base images. Tags are not accepted because they can move between preparation and image creation.

```sh
magelift build --push \
  --image ghcr.io/example/shop:candidate \
  --builder-image ghcr.io/magelift/magelift-builder@sha256:BUILDER_DIGEST \
  --runtime-image ghcr.io/magelift/magelift-runtime@sha256:RUNTIME_DIGEST
```

MageLift builds `linux/amd64` and `linux/arm64` by default. Use `--platform` to choose a smaller tested set.

The Git `origin` must be an HTTPS URL. Repositories with an SSH origin can pass the public source URL explicitly:

```sh
magelift build --push \
  --source-url https://github.com/example/shop.git \
  --image ghcr.io/example/shop:candidate \
  --builder-image ghcr.io/magelift/magelift-builder@sha256:BUILDER_DIGEST \
  --runtime-image ghcr.io/magelift/magelift-runtime@sha256:RUNTIME_DIGEST
```

The CLI adds the full inspected commit checksum to the provenance URL. It rejects URL credentials and unrelated query parameters so they cannot enter image labels or attestations.

BuildKit publishes an SBOM and SLSA provenance with the image. MageLift then signs the pushed digest. `magelift build --push` and `magelift sign` use the current `gcloud` or CI login; GCP impersonates the Magento CI service account from `magelift bootstrap` so operators never pass Cosign flags. Identity tokens are passed to Cosign only as a file path (never a JWT on argv). CR/LF is stripped first. Advanced automation may still set `MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE`, `MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV`, or `--identity-token-file`. With no Magelift or `gcloud` token source, Cosign uses ambient OIDC (GitHub Actions or a one-time browser confirmation). Artifact Registry, ECR, and Binary Authorization signatures are not substitutes. The release workflow pins Cosign 3.0.6 through the verified installer action.

Composer credentials are mounted as a private file during preparation. They are not passed to BuildKit, copied into the image, or written to the artifact manifest.

Before BuildKit creates the application image, MageLift compares the isolated
builder's reported PHP branch, Composer version, and loaded extensions with the
requested build contract. A builder response that does not satisfy the request
stops the build before image creation; the `opcache` request is satisfied by the
PHP runtime's `Zend OPcache` module name, which is normalized to the stable
`zend_opcache` artifact identifier.

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
