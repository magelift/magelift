# ece-tools parity matrix

Honesty surface for MageLift vs Adobe Commerce Cloud / ece-tools operator
expectations (ECE-01, D-06). Every row is either `closed` (behavior shipped and
verifiable in this repo) or `intentional-gap` (explicitly not claimed). Blank
status cells are forbidden.

This is **not** a full ece-tools behavioral clone. Claims below match what Phase 4
shipped in plans 04-03 (import allowlist), 04-04 (SCD strategy/threads), 04-05
(m2-hotfixes), and the Magento patch lifecycle change. Public Adobe documentation
is cited for product shape only; no sibling PaaS source is vendored (IMPORT-06 /
[provenance](provenance.md)).

Field shapes for portable Magento inputs live in
[configuration](configuration.md) (`build.staticContent.strategy` /
`threads`, `build.extensions`, `build.composer.version`, `application.cron`,
typed `build.hooks`).

Status vocabulary: `closed` | `intentional-gap`.

## Build phase (prepare / image)

| Capability | Status | Notes |
| --- | --- | --- |
| `composer validate --strict` | closed | `LifecyclePlan` validate phase (`build/src/Magento/LifecyclePlan.php`). |
| `composer check-platform-reqs --no-dev` | closed | Same validate phase as ece-tools-style platform gate. |
| `composer install --no-dev` (prefer-dist, optimize-autoloader) | closed | Build phase before patches and Magento argv. |
| PHP runtime extension declaration | closed | `build.extensions` is checked inside the isolated builder before lifecycle execution; ACC and Upsun `runtime.extensions` entries are imported. |
| Composer version declaration | closed | `build.composer.version` is checked against the isolated builder; ACC and Upsun `dependencies.php.composer/composer` entries are imported. |
| Custom `m2-hotfixes/*.patch` apply (alpha order, after `composer install`) | closed | `PatchLifecycle` delegates the complete sequence to the project-installed Cloud Patches executable when available; otherwise the clean-room `PatchApplier` fallback uses host `patch -p1` with idempotent dry-run checks. |
| `QUALITY_PATCHES` / cloud-required Quality Patches Tool IDs | closed | `PatchLifecycle` invokes the project-installed `ece-patches` or standalone `magento-patches` executable; unsupported IDs remain upstream failures. MageLift does not vendor the Quality Patches database. |
| `setup:di:compile` during prepare | closed | Emitted after composer install and hotfixes in build phase. |
| `setup:static-content:deploy` locale × theme matrix | closed | `build.staticContent.locales` / `themes` → prepare protocol → Magento SCD argv with `--force`. |
| SCD strategy (`SCD_STRATEGY` / `-s`) | closed | `build.staticContent.strategy` → `-s` per locale×theme (ECE-03 / 04-04). Values: quick, standard, compact. |
| SCD threads (`SCD_THREADS` / `-j`) | closed | `build.staticContent.threads` (≥1) → `-j` per locale×theme (ECE-03 / 04-04). |
| SCD when locales/themes are configured | closed | MageLift emits forced `setup:static-content:deploy` commands for the explicit locale × theme matrix. |
| SCD without dumped store configuration | closed | The build phase omits SCD when no matrix is configured because the isolated builder has no Magento database. |
| Free-form PaaS `hooks.build` shell scripts | intentional-gap | MageLift accepts typed `build.hooks` command vectors (`composer` / `magento` only); shell strings are rejected and importers treat free-form hooks as unmapped. |
| Magento Cloud `bin/magento` balance / module enable cloud helpers | intentional-gap | Not part of the prepare DAG; operators use typed hooks or day-2 CLI. |
| OCI packaging after prepare | closed | Go build pipeline owns package; PHP plan exposes a stable package hand-off node. |

## Deploy phase

| Capability | Status | Notes |
| --- | --- | --- |
| `app:config:import` | closed | Deploy phase Magento argv in `LifecyclePlan`. |
| `setup:upgrade --keep-generated` | closed | Deploy phase after config import. |
| `cache:clean` after upgrade | closed | Deploy phase. |
| Free-form PaaS `hooks.deploy` shell scripts | intentional-gap | Deploy-time typed hooks are provider extension concerns; PaaS shell hooks are unmapped on import. |
| Zero-downtime blue/green Magento Cloud deploy semantics | intentional-gap | MageLift uses candidate ECS task + promote; not an ece-tools clone. |
| Dump / media cutover during deploy | intentional-gap | Phase 5 owns dump/media; not claimed here. |

## Post-deploy phase

| Capability | Status | Notes |
| --- | --- | --- |
| `cache:flush` | closed | Post-deploy Magento argv in `LifecyclePlan`. |
| Free-form PaaS `hooks.post_deploy` shell scripts | intentional-gap | Same typed-hook policy as deploy; not imported as shell. |
| Warm-up pages / indexers as ece-tools post-deploy extras | intentional-gap | Operators use day-2 `reindex` / custom hooks; not default post-deploy. |

## D-07 env and relationship allowlist (ECE-04)

Frozen v1 importer allowlist. Mapped keys become reviewable `magelift.yaml`;
everything else is reported in `magelift.unmapped.md` (or stem sidecar) with
non-zero exit; never a silent drop (D-05 / 04-03).

| PaaS shape | Status | Maps to |
| --- | --- | --- |
| `CRYPT_KEY` (stage / variables) | closed | `target.aws.encryptionKeySecretArn` placeholder ARN only; plaintext crypt never written (`internal/paasimport`). |
| `UPDATE_URLS` | closed | `environments.staging.domain` (host stripped from URL). |
| Routes host (non-`{default}` pattern) | closed | `environments.staging.domain` when no `UPDATE_URLS`. |
| `SCD_STRATEGY` | closed | `build.staticContent.strategy`. |
| `SCD_THREADS` | closed | `build.staticContent.threads` (integer ≥1; invalid values → unmapped). |
| DB / MySQL / MariaDB relationship | closed | Recognized as supported shape; runtime injects DB via capability / `MAGENTO_DC_*` seams (no raw env dump). |
| Redis (cache / session) relationship | closed | Recognized; runtime Magento env from cache capability. |
| OpenSearch / Elasticsearch relationship | closed | `target.aws.catalog.searchMode` intent (e.g. serverless) + runtime search env. |
| RabbitMQ / AMQP relationship | closed | `target.aws.catalog.queueMode` (e.g. ecs-rabbitmq) + runtime queue env. |
| Magento `cron:run` crontab entries | closed | `application.cron` list of `{schedule, command}`. |
| Free-form shell crontab entries | intentional-gap | Reported unmapped; not rewritten into portable YAML. |
| Long-tail stage / variables env beyond this allowlist | intentional-gap | Examples: `MYSQL_USE_SLAVE_CONNECTION`, `REDIS_BACKEND`, `NPROC`, `PHP_*` tuning, `SKIP_HTML_MINIFICATION`, `CLEAN_STATIC_FILES`, `WARM_UP_PAGES`, `SCD_MATRIX`, `SCD_MAX_EXEC_TIME`, `ERROR_REPORT_DIR_NESTING_LEVEL`, `ENABLE_EVENTING`, Xdebug flags. Appear in unmapped sidecar; operators edit `magelift.yaml` by hand. |
| Dumping raw `MAGENTO_DC_*` blocks into YAML | intentional-gap | Runtime adapters already emit `MAGENTO_DC_*` from capabilities; importers map relationships/intent only. |

## Operator notes

- Importers: `magelift init --from-acc` / `--from-upsun` (see [migrating from PaaS](migrating-from-paas.md)).
- Foreign PaaS YAML is never valid `--config` input: `config.Load` requires
  `schemaVersion: 1`.
- Clean-room policy: [provenance](provenance.md) and
  `make check-clean-room`.
