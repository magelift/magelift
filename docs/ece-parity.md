# ece-tools parity matrix

Honesty surface for MageLift vs Adobe Commerce Cloud / ece-tools operator
expectations (ECE-01, D-06). Every row is either `closed` (behavior shipped and
verifiable in this repo) or `intentional-gap` (explicitly not claimed). Blank
status cells are forbidden.

This is **not** a full ece-tools behavioral clone. Claims below match what Phase 4
shipped in plans 04-03 (import allowlist), 04-04 (SCD strategy/threads), and
04-05 (m2-hotfixes). Public Adobe documentation is cited for product shape only;
no sibling PaaS source is vendored (IMPORT-06 / [provenance](provenance.md)).

Field shapes for portable Magento inputs live in
[configuration](configuration.md) (`build.staticContent.strategy` /
`threads`, `application.cron`, typed `build.hooks`).

Status vocabulary: `closed` | `intentional-gap`.

## Build phase (prepare / image)

| Capability | Status | Notes |
| --- | --- | --- |
| `composer validate --strict` | closed | `LifecyclePlan` validate phase (`build/src/Magento/LifecyclePlan.php`). |
| `composer check-platform-reqs --no-dev` | closed | Same validate phase as ece-tools-style platform gate. |
| `composer install --no-dev` (prefer-dist, optimize-autoloader) | closed | Build phase before patches and Magento argv. |
| Custom `m2-hotfixes/*.patch` apply (alpha order, after `composer install`) | closed | Clean-room `MageLift\Build\Magento\PatchApplier` via host `patch -p1`. See `build/src/Magento/PatchApplier.php` (ECE-02 / 04-05). |
| `QUALITY_PATCHES` / cloud-required Quality Patches Tool IDs | intentional-gap | No Adobe quality-patch database is vendored (IMPORT-06). Selecting QPT IDs is not implemented. |
| `setup:di:compile` during prepare | closed | Emitted after composer install and hotfixes in build phase. |
| `setup:static-content:deploy` locale × theme matrix | closed | `build.staticContent.locales` / `themes` → prepare protocol → Magento SCD argv. |
| SCD strategy (`SCD_STRATEGY` / `-s`) | closed | `build.staticContent.strategy` → `-s` per locale×theme (ECE-03 / 04-04). Values: quick, standard, compact. |
| SCD threads (`SCD_THREADS` / `-j`) | closed | `build.staticContent.threads` (≥1) → `-j` per locale×theme (ECE-03 / 04-04). |
| Default SCD when locales/themes unset | closed | Single `setup:static-content:deploy --no-interaction` with no `-s`/`-j`. |
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
