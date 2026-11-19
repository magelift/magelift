# Application builds

`magelift build` prepares Magento in the pinned builder image and creates an application image from the result. The source repository must be clean. MageLift writes the final artifact manifest outside the repository.

For a local build, run:

```sh
magelift build
```

Run from the shop repository, with `magelift.php` and `composer.lock` committed.
Existing `app/etc/config.php` is an immutable input; disabled modules stay disabled.
The bundled nginx-fpm recipe supports PHP 8.3, 8.4, and 8.5 on `linux/amd64`, including on an ARM workstation. MageLift selects the branch from `build.php` for both builder and runtime images. The CLI ships the
existing Docker and PHP build recipes with pinned upstream image digests.
It builds the builder and runtime images on your Docker engine, then builds
the application. BuildKit can reuse its cache. No MageLift source checkout or
MageLift image registry is required. Native ARM runtime qualification remains
NOT_RUN.

The nginx runtime maps Magento's numeric static-version URL prefix to the same
prebuilt files under pub/static, including files whose path contains a vendor
segment. It serves missing static assets as 404 and keeps script and
private-media denials. A root /vendor request stays denied. Updating the
bundled runtime recipe requires rebuilding the user's application image
through CI; promoting an older digest does not apply the new nginx
configuration.

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

BuildKit publishes the SBOM and SLSA provenance. MageLift signs the pushed digest with Cosign. Give the GitHub build job `id-token: write` so it can sign with its job identity. On a workstation, use your current Google login and the bootstrapped CI service account, or complete Cosign’s browser login. For automation, use `MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE`, `MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV` or `--identity-token-file` to supply a token explicitly. MageLift passes Cosign a file path and keeps the token out of the process arguments.

### Composer

The bundled builder uses Composer 2.11.2. An older shop can usually keep its existing `composer.lock` and use the bundled binary. Check that its plugins accept the builder’s Composer plugin API. Set `build.composer.version` to a minimum (`2.9.8+`), a minor branch (`2.10`), a patch range within one branch (`~2.9.8`), or one release (`2.11.2`). If a plugin requires an older binary, set both its version constraint and `build.composer.image` to match. The image must run `/usr/bin/composer --version` and is selected by digest. When you supply a complete builder with `--builder-image`, MageLift checks the Composer installed in that builder. [Composer platform dependencies](https://getcomposer.org/doc/articles/composer-platform-dependencies.md)

Before reading the shop’s Composer credentials, `magelift build` queries OSV with the Composer package name and the version in the selected builder. A reported critical advisory stops the build. High-severity findings produce a warning. If OSV is unavailable or cannot classify a finding, the CLI says which check was incomplete. A feed can lag the upstream release notes. Use `composer audit --locked` to review advisories for the shop’s locked packages. [OSV package queries](https://google.github.io/osv.dev/post-v1-query/) · [Composer security releases](https://getcomposer.org/changelog/2.11.2) · [Composer audit](https://getcomposer.org/doc/03-cli.md#audit)

During preparation, the runner mounts Composer credentials as a private file for the shop’s dependency install.

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

```php
[
    'build' => [
        'hooks' => [
            'build.prepare' => [
                'phase' => 'build',
                'relationship' => 'before',
                'target' => 'build',
                'command' => [
                    'executable' => 'composer',
                    'arguments' => ['run-script', 'prepare'],
                ],
            ],
        ],
    ],
]
```

Hooks are ordered by their target and dependency edges. Retries require
`idempotent: true`, and each hook has a bounded timeout. Project hooks are limited to
the validate, build, and package phases; deployment-time hooks are provider
extension concerns until the deployment protocol exposes the same typed contract.

## Inspect the recipe

```sh
magelift build --export-recipe .magelift/recipe-inspection
```

The destination must not exist. This exports the versioned files and reports
the recipe SHA256 without invoking Docker, Composer credentials or deployment.
Generated files are diagnostic output; keep configuration in the PHP project. Source-built
images carry `io.magelift.recipe.sha256`; their runtime label uses
`recipe:sha256:<hash>` rather than claiming a separately published runtime digest.
Explicit base-image overrides retain their digest labels.

## Optional private registry cache

```php
[
    'build' => [
        'cache' => [
            'from' => ['us-central1-docker.pkg.dev/example/build-cache/shop:amd64-recipe-release'],
            'to' => 'us-central1-docker.pkg.dev/example/build-cache/shop:amd64-recipe-release',
            'mode' => 'max',
            'ignoreExportErrors' => false,
        ],
    ],
]
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

```php
[
    'build' => [
        'composer' => [
            'credentials' => 'env://COMPOSER_AUTH',
        ],
    ],
]
```

The CI supplies `COMPOSER_AUTH` as a secret containing a non-empty JSON object.
An absent or invalid value fails without printing its contents. This reference
is available only for build-time Composer credentials; runtime secrets retain
the provider reference contract. The existing cloud secret references remain
available. Never put the credential JSON in the PHP project or pass it as a build argument.

## Patch application

By default, declared `build.qualityPatches` IDs run through the standalone
Quality Patches executable from the production Composer lock, then sorted
`m2-hotfixes` through the host `patch` tool with idempotent dry-run checks.

Optional `build.cloudPatches: true` opts into one upstream `ece-patches apply`
after `composer install`. The shop must lock `magento/magento-cloud-patches`;
declared Quality Patch IDs also require `magento/quality-patches` in the
production lock. Only project-declared Quality Patch IDs control selection;
leftover `.magento.env.yaml` or ambient environment values are ignored. Cloud-required patches, those IDs,
and existing `m2-hotfixes` are applied in the single upstream pass; MageLift
does not run a second Quality Patch or host hotfix stage afterward. Upstream
owns applicability and patch databases; this path is not a certified matrix cell.

`magelift build` prints a scope notice on stderr when cloud patches are enabled.

## Hotfix failures

Host hotfix application first checks whether a patch is needed or fully applied.
A conflict or partially applied patch stops preparation; review its applicability
against the locked Magento version before adjusting it and retrying. These
checks do not merge conflicting patches or make a whole patch series atomic.
When `build.cloudPatches` is enabled, hotfix failures surface through the
upstream apply instead of the host `patch` fallback.
