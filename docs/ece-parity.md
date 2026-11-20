# Adobe Commerce Cloud build and deployment settings

When moving a shop that uses ece-tools, use the importer to draft the project
settings. Then compare the shop's existing build and deployment hooks with the
steps below. [Move a Magento shop to MageLift](migrating-from-paas.md)

## Build an image

MageLift checks the configured PHP extensions and Composer version inside
the builder. It validates `composer.json` and checks platform requirements
before installing the locked dependencies. It applies the shop's patches
before running dependency injection compilation.

Adobe Commerce Cloud projects can use the Cloud Patches executable installed
by the shop. MageLift also uses the shop's installed Quality Patches command
for configured patch IDs. Custom patches in `m2-hotfixes/` run in filename
order. When the Cloud Patches executable is unavailable, MageLift can apply
custom patches with `patch -p1`.

Set `build.staticContent.locales` and `build.staticContent.themes` to generate
assets at build time from the shop's dumped store configuration.
`build.staticContent.strategy` accepts `quick`, `standard` or `compact`, and
`build.staticContent.threads` sets the worker count. A theme with an empty
locale list in `build.staticContent.matrix` produces no static assets.
[Application builds](builds.md)

## Deploy the candidate

MageLift runs `setup:upgrade --keep-generated`, imports Magento configuration
and cleans caches. It flushes caches after deployment. Rehearse database and
media transfer separately. Include writes made during cutover in the rehearsal.
[Deployment operations](operations.md) · [Data migration](weekend-migrate.md)

Review shell hooks and scheduler entries from the source platform. Translate
build steps that call Composer or Magento into `build.hooks` command arrays.
Schedule any remaining jobs separately. Run page warm-up and reindexing when
the shop needs them.

## Check imported settings

| Source setting | MageLift project |
| --- | --- |
| `CRYPT_KEY` | Recreate the original key in your cloud secret store and set the destination's encryption key reference. The importer uses a placeholder and keeps the key value out of the project. |
| `UPDATE_URLS` or a concrete route hostname | `environments.staging.domain`. |
| `SCD_STRATEGY` and `SCD_THREADS` | `build.staticContent.strategy` and `build.staticContent.threads`. |
| `QUALITY_PATCHES` | The shop's build patch settings. |
| Admin front name, cookie domain, CORS and consumer runner settings | The Magento settings under `application.magento`. |
| Database, cache, session, search and queue relationships | Service choices and provider-specific options. MageLift supplies runtime connection details to Magento. |
| Magento `cron:run` entries | `application.cron`. |

The import report names source keys that need a decision, including custom
mounts, shell hooks and other environment variables. Review the report alongside
the source configuration before deploying. [Project settings](configuration.md)
