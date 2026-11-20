# Application builds

`magelift build` prepares Magento in a pinned builder image and packages the
result as an application image. Run it from a clean shop checkout with
`magelift.php` and `composer.lock` committed. MageLift writes the artifact
manifest outside the repository.

```sh
magelift build
```

MageLift reads the existing `app/etc/config.php` and preserves disabled modules.
The CLI bundles an nginx/PHP-FPM recipe for PHP 8.3, 8.4 and 8.5 on
`linux/amd64`, including builds from an ARM workstation. MageLift uses
`build.php` to select the PHP branch for both the builder and the runtime.
BuildKit uses the pinned upstream image digests and can reuse its cache.

nginx maps Magento's numeric static-version URL prefix to the prebuilt files
under pub/static, including paths with a vendor segment. Missing assets return
404. nginx runs only approved PHP entry points and denies private media and
requests to /vendor at the root.
Rebuild the application image in CI to pick up changes to the bundled nginx
configuration. Each digest uses the configuration baked into that image.

MageLift puts temporary BuildKit bind mounts under the user cache so Docker
Desktop, Colima and other engines with access to `/Users` can read them. It uses
the active Docker Buildx builder. Set `BUILDX_BUILDER` to select a named builder
such as `colima`.

## Push a release candidate

Run this command in your CI/CD after authenticating Docker to your registry.
BuildKit runs the builder and runtime stages on the runner. MageLift publishes
the finished application image to the repository you selected.

```sh
magelift build --push --image ghcr.io/example/shop:candidate
```

The pushed build manifest records the registry and immutable image digest.
Use the manifest path printed by the build as the value of `deploy --from-build`.
A manifest from a local build has no registry digest. If you already have a
pushed image and an older manifest, pass the full image reference to
`deploy --digest`.

For ECR, create the application repository and grant CI push/pull permissions.
GHCR, ECR, Artifact Registry and other OCI registries use the same `--image`
option and Docker's configured credentials. For example,
`123456789012.dkr.ecr.us-east-1.amazonaws.com/shop:candidate` selects ECR.
Grant the deployment runtime its own persistent pull identity and check that
it can pull the image after CI pushes it.

A local `magelift build` loads images into Docker without publishing them.
Pushed builds default to `linux/amd64`. Supply `--builder-image` and
`--runtime-image` overrides must be supplied together. Pushed overrides require
registry digests. Explicit ARM or multiple-platform builds require those
custom builder and runtime images.

Pushed builds require an immutable HTTPS provenance URL. You can use a private
repository or a detached checkout without setting a local branch or author
identity. For a repository with an SSH remote, pass the HTTPS source URL.

```sh
magelift build --push \
  --source-url https://github.com/example/shop.git \
  --image ghcr.io/example/shop:candidate
```

The CLI adds the full inspected commit checksum to the provenance URL. It rejects URL credentials and unrelated query parameters so they cannot enter image labels or attestations.

BuildKit publishes the SBOM and SLSA provenance. MageLift signs the pushed digest with Cosign. Give the GitHub build job `id-token: write` so it can sign with its job identity. On a workstation, use your current Google login and the bootstrapped CI service account, or complete Cosign’s browser login. For automation, use `MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE`, `MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV` or `--identity-token-file` to supply a token explicitly. MageLift passes Cosign a file path and keeps the token out of the process arguments.

### Static assets by theme

Use `build.staticContent.matrix` when the Admin and storefront themes need different locales. In the PHP project, a theme with an empty list produces no static assets. The builder runs Magento for each listed theme and locale. You can also use `locales` and `themes` together to build every combination.

```php
'staticContent' => [
    'matrix' => [
        'Magento/backend' => ['en_US'],
        'Example/store' => ['en_US', 'fr_FR'],
        'Magento/blank' => [],
    ],
    'threads' => 2,
],
```

The PaaS importer reads `SCD_MATRIX` and `QUALITY_PATCHES` from the Magento build stage. It preserves patch IDs for the shop's installed patch tooling. Keep custom `m2-hotfixes` files in version control along with the lockfile. [Adobe build variables](https://experienceleague.adobe.com/en/docs/commerce-on-cloud/user-guide/configure/env/stage/variables-build)

### Composer

The bundled builder uses Composer 2.11.2. An older shop can usually keep its existing `composer.lock` and use the bundled binary. Check that its plugins accept the builder’s Composer plugin API. Set `build.composer.version` to a minimum (`2.9.8+`), a minor branch (`2.10`), a patch range within one branch (`~2.9.8`), or one release (`2.11.2`). If a plugin requires an older binary, set both its version constraint and `build.composer.image` to match. The image must run `/usr/bin/composer --version` and is selected by digest. When you supply a complete builder with `--builder-image`, MageLift checks the Composer installed in that builder. [Composer platform dependencies](https://getcomposer.org/doc/articles/composer-platform-dependencies.md)

Before reading the shop’s Composer credentials, `magelift build` queries OSV with the Composer package name and the version in the selected builder. A reported critical advisory stops the build. High-severity findings produce a warning. If OSV is unavailable or cannot classify a finding, the CLI says which check was incomplete. A feed can lag the upstream release notes. Use `composer audit --locked` to review advisories for the shop’s locked packages. [OSV package queries](https://google.github.io/osv.dev/post-v1-query/) · [Composer security releases](https://getcomposer.org/changelog/2.11.2) · [Composer audit](https://getcomposer.org/doc/03-cli.md#audit)

During preparation, the runner mounts Composer credentials as a private file for the shop’s dependency install.

Before BuildKit creates the application image, MageLift checks the builder's
PHP branch, Composer version and loaded extensions against the project. A
mismatch stops the build. The PHP module `Zend OPcache` satisfies an `opcache`
request and appears as `zend_opcache` in the artifact.

## Traceability

The external manifest records the source revision, locked input
hashes, PHP and Composer versions, modules, static-content matrix and final
image digest. It records the CLI version as `0.0.0-dev` for development builds.
The build package has its own version.

The image carries `io.magelift.builder.image` and the recipe fingerprint in
`io.magelift.recipe.sha256`. The manifest records that fingerprint, platform and
pinned upstream bases. Advanced base overrides retain their runtime identity
label. The final digest links these inputs to the external manifest. Repeat builds may have different OCI digests while
retaining equivalent declared inputs and application content.

## Preparation hooks

Use hooks to run Composer or `bin/magento` during preparation. Pass the
executable and arguments separately.

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

MageLift orders hooks by target and dependencies. Each hook has a timeout,
and retries require `idempotent: true`. Project hooks run during validate,
build or package. Use a provider extension for deploy-time hooks.

## Inspect the recipe

```sh
magelift build --export-recipe .magelift/recipe-inspection
```

Choose a destination that does not exist. The command writes the versioned
recipe files and reports the SHA-256 without starting Docker, reading
Composer credentials or deploying. Keep configuration in the PHP project.
Images built from the bundled recipe carry `io.magelift.recipe.sha256` and a
runtime label of `recipe:sha256:<hash>`. Explicit base-image overrides
retain their digest labels.

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

Give the cache its own registry reference. Scope it by project, architecture,
recipe and who can access it. Use separate references and permissions for
untrusted builds. Cached layers can contain private source and packages, so
keep the cache repository private and give it a retention policy.

Set `ignoreExportErrors` to continue after a cache export fails. Build and
image publication errors still stop the command. A missing cache triggers a
fresh build. The registry stores intermediate layers in max mode. On a new
runner, expect to rebuild the contents of `RUN --mount=type=cache` directories.

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
MageLift reports a missing or invalid value without printing it. Use this
reference for Composer during builds. Runtime secrets use the provider's
reference format. You can also read supported cloud secret references for
Composer. Keep the credential JSON in the secret store. Use its reference in
the project and keep the value out of build arguments.

## Patch application

By default, declared `build.qualityPatches` IDs run through the standalone
Quality Patches executable from the production Composer lock, then sorted
`m2-hotfixes` through the host `patch` tool with idempotent dry-run checks.

Optional `build.cloudPatches: true` opts into one upstream `ece-patches apply`
after `composer install`. Lock `magento/magento-cloud-patches` in the shop's
production dependencies. If you declare Quality Patch IDs, also lock
`magento/quality-patches`. MageLift takes those IDs from the project.
`.magento.env.yaml` and environment variables do not affect patch selection.
The upstream command applies the required Cloud Patches, selected Quality
Patches and `m2-hotfixes` in one pass. Upstream determines which patches apply
and owns the patch database.

`magelift build` prints a scope notice on stderr when cloud patches are enabled.

## Hotfix failures

Host hotfix application first checks whether a patch is needed or fully applied.
A conflict or partially applied patch stops preparation. Check the patch
against the locked Magento version before adjusting it and retrying. These
checks do not merge conflicting patches or make a whole patch series atomic.
With `build.cloudPatches` enabled, check the upstream patch output for hotfix failures.
