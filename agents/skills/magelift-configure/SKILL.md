---
name: magelift-configure
description: Set up magelift.php, choose Magento service versions, validate an environment, or generate the shop's GitHub Actions workflow.
version: 1.0.0
---

# Configure a shop

Run `magelift init --provider aws` from the Magento checkout. Use `gcp` or
`azure` for the other clouds. The CLI reads the edition and version from
the Composer lockfile and writes `magelift.php` with staging, UAT, production, and
preview environments. For an Adobe Commerce Cloud, Upsun, or Compose shop,
use the corresponding `--from-*` importer.

Set shared service versions once. Change capacity and cloud-specific options
in the environment that needs them. Keep previews on separate data and
credentials unless the shop owner requests shared resources. Review all
PHP project files before executing them. Commit any generated
`magelift.environments.php` needed by CI.

```sh
magelift compatibility catalog 2.4.8-p5
magelift config validate --env staging
magelift config effective --env staging
```

Use `magelift env diff` with two environment names to check overrides.
The compatibility command reads a dated Adobe requirements snapshot.
Review its warnings and check whether the cloud can supply the selected
service in the shop's region. An existing service needs its endpoint and
any required secret reference. [Service settings](https://magelift.dev/docs/service-bindings/)

## Build and review the plan

Commit the project, Composer lock, and existing module configuration before
building. The bundled nginx and PHP-FPM recipe uses the PHP version selected
in `build.php` for versions 8.3 through 8.5 on `linux/amd64`. It includes
Composer. Set `build.composer.image` to a verified image digest if a locked
plugin needs a different Composer version.

Use `env://COMPOSER_AUTH` or an available cloud secret reference for private
Composer dependencies. Build with `magelift build --push --image` to publish
the application to the shop's registry. Pass the printed manifest path to
`magelift deploy --from-build`. Use the registry digest when a workflow
expects `--digest`. Rebuild after an update to the bundled recipe.
[Application builds](https://magelift.dev/docs/builds/)

## Generate a shop workflow

Use `magelift ci generate --magelift-version vX.Y.Z` with a published
release. If the existing workflow differs, review the candidate before
using `--replace`. Use `magelift ci validate` to check the config and YAML
syntax. `--check-template` also checks the generated template.

Configure the build and deploy identities for each GitHub environment.
Apply `magelift-preview` only after reviewing the current pull-request
commit. The build can read the shop's configured Composer credentials.
Remove and reapply the label after reviewing a new commit. Removing the
label or closing the pull request schedules teardown.
[Cloud setup](https://magelift.dev/docs/bootstrap/)
