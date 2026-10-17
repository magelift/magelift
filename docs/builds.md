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
MageLift constructs the builder and runtime stages on the runner and publishes
the deployable application to your selected repository. Intermediate images do
not need to be published. The runtime is built from the bundled recipe in the
same BuildKit build as the final application; no MageLift image registry is used.

```sh
magelift build --push --image ghcr.io/example/shop:candidate
```

For ECR, create the application repository and grant CI push/pull permissions.
GHCR, ECR, Artifact Registry and other OCI registries use the same `--image`
option and Docker's configured credentials. For example,
`123456789012.dkr.ecr.us-east-1.amazonaws.com/shop:candidate` selects ECR.
Successful publication does not prove that the deployment runtime can pull the
image; configure and test its own durable pull identity separately.

A local `magelift build` loads images into Docker without publishing them.
Pushed builds also default to `linux/amd64`. Advanced `--builder-image` and
`--runtime-image` overrides must be supplied together. Pushed overrides require
registry digests. Explicit ARM or multiple-platform builds require those
advanced images and remain outside the community alpha qualification.

Pushed builds require an immutable HTTPS provenance URL. Private repositories
and detached checkouts are accepted; no local branch or Git author identity is
required. Repositories with an SSH origin can pass their HTTPS source URL:

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

The image carries `io.magelift.builder.image` and the recipe fingerprint in
`io.magelift.recipe.sha256`. The manifest records that fingerprint, platform and
pinned upstream bases. Advanced base overrides retain their runtime identity
label. The final digest links these inputs to the external manifest. Repeat builds may have different OCI digests while
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

## Inspect the recipe

```sh
magelift build --export-recipe .magelift/recipe-inspection
```

The destination must not exist. This exports the versioned files and reports
the recipe SHA256 without invoking Docker, Composer credentials or deployment.
Generated files are diagnostic output; keep configuration in YAML. Source-built
images carry `io.magelift.recipe.sha256`; their runtime label uses
`recipe:sha256:<hash>` rather than claiming a separately published runtime digest.
Explicit base-image overrides retain their digest labels.

## Optional private registry cache

```yaml
build:
  cache:
    from: [us-central1-docker.pkg.dev/example/build-cache/shop:amd64-recipe-release]
    to: us-central1-docker.pkg.dev/example/build-cache/shop:amd64-recipe-release
    mode: max
    ignoreExportErrors: false
```

Cache references are separate from the final image. Scope them by project,
architecture/recipe and trust boundary; use different references and permissions
for untrusted work. Cached layers may contain private source and proprietary
packages. Keep that repository private and apply its own retention policy.
`ignoreExportErrors` is an explicit opt-in; publication/build errors remain fatal.
A missing cache falls back to a cold build. Registry export retains layers,
including intermediate stages in max mode; it does not promise to restore the
contents of `RUN --mount=type=cache` directories on an ephemeral runner.

## Composer credentials supplied by CI

```yaml
build:
  composer:
    credentials: env://COMPOSER_AUTH
```

The CI supplies `COMPOSER_AUTH` as a secret containing a non-empty JSON object.
An absent or invalid value fails without printing its contents. This reference
is available only for build-time Composer credentials; runtime secrets retain
the provider reference contract. The existing cloud secret references remain
available. Never put the credential JSON in YAML or pass it as a build argument.

## Hotfix failures

The existing lifecycle applies declared Quality Patch IDs through the tool
installed by the shop's production Composer lock, then sorted `m2-hotfixes`.
Host hotfix application first checks whether a patch is needed or fully applied.
A conflict or partially applied patch stops preparation; review its applicability
against the locked Magento version before adjusting it and retrying. These
checks do not merge conflicting patches or make a whole patch series atomic.
