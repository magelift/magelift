# Application builds

`magelift build` prepares Magento in the pinned builder image and creates an application image from the result. The source repository must be clean. MageLift writes the final artifact manifest outside the repository.

For a local build, run:

```sh
magelift build
```

The local build uses the host platform and the pinned MageLift images already loaded in Docker.

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

BuildKit publishes an SBOM and SLSA provenance with the image. MageLift then signs the pushed digest with Cosign keyless signing. The release workflow pins Cosign 3.0.6 through the verified installer action; local callers need a current Cosign release and a supported OIDC identity.

Composer credentials are mounted as a private file during preparation. They are not passed to BuildKit, copied into the image, or written to the artifact manifest.

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
