# Migrating from Adobe Commerce Cloud or Platform.sh

If you already ship Magento on Adobe Commerce Cloud (ACC) or Platform.sh /
Upsun, MageLift should feel familiar at the edges and different in the middle.
You still declare environments in YAML, promote an immutable build, and use
CLI verbs for logs and shells. The cloud account, network, and managed services
are yours — MageLift does not rent a PaaS for you.

Certified path today: AWS ECS Fargate. Experimental targets are labeled and are
not ACC replacements yet.

## Mental model

| ACC / Platform.sh idea | MageLift equivalent |
| --- | --- |
| `.magento.app.yaml` / `.platform.app.yaml` | `magelift.yaml` (portable Magento inputs) |
| Environment branches / multi-env | `environments.*` overlays with inheritance |
| Variables / secrets | Secret references (`aws-secrets-manager://…`), never plaintext |
| Build + deploy hooks | Build pipeline + post-deploy Magento phases |
| `magento-cloud` / `platform` SSH | `magelift exec` / `magelift ssh` (ECS Exec into web or cron) |
| Environment logs | `magelift logs --service web\|deploy\|cron` |
| Cron workers | ECS cron service + `magelift cron-run` |
| Local Lando / Docker | `magelift dev` Compose loop (account-free) |

## What stays the same

- One config file in the Magento repo drives environments.
- Builds produce a digest you promote; you do not rebuild for production.
- Day-2 ops are CLI-first: health, logs, exec, cache flush, reindex.
- Preview / staging / production are first-class environment classes.

## What changes on purpose

- **You own the AWS account.** Billing, IAM, DNS, and KMS keys are yours.
  Bootstrap creates the shared control-plane pieces; stacks stay in that account.
- **Topology is not YAML soup.** Network CIDRs, Aurora vs RDS, OpenSearch mode,
  and NAT mode are explicit escape hatches on `target.aws`, not hidden SKU swaps.
- **No inbound SSH.** `magelift ssh` is ECS Exec. There is no bastion and no
  `magelift tunnel` on the Fargate path.
- **Deploy is not a shell service.** Migrations run as a one-off candidate ECS
  task. Use `magelift logs --service deploy` for migrate output. `exec` only
  attaches to long-lived `web` or `cron` tasks.
- **Build cannot differ per environment.** Artifacts are immutable; environment
  overlays change runtime config and capacity, not the image.

## Side-by-side: ACC app YAML → MageLift

ACC / Upsun style (illustrative):

```yaml
# .magento.app.yaml (shape only — do not paste into MageLift)
name: app
type: php:8.3
hooks:
  build: |
    set -e
    composer install
  deploy: |
    php bin/magento setup:upgrade
relationships:
  database: "mysql:mysql"
  redis: "redis:redis"
  rabbitmq: "rabbitmq:rabbitmq"
```

MageLift equivalent (portable inputs + AWS catalog escape hatches):

```yaml
schemaVersion: 1
project:
  name: app
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
build:
  php: "8.5"
  composer:
    credentials: aws-secrets-manager://magelift/composer
target:
  provider: aws
  runtime: ecs-fargate
  aws:
    catalog:
      queueMode: ecs-rabbitmq   # or amazon-mq | db
defaults:
  region: eu-west-3
  preset: preview
environments:
  staging:
    account: "123456789012"
    branches: [main]
    domain: staging.example.com
```

Hooks stay typed Magento/build phases — not free-form shell strings in portable YAML.
Variables become secret references. Topology (Aurora vs RDS, queue mode, NAT) lives
under `target.aws`, not as PaaS relationship aliases.

## Generate YAML from ACC / Upsun (shipped)

Offline importers **generate** reviewable MageLift YAML. They never accept foreign
PaaS schemas as deploy input: `magelift --config .magento.app.yaml …` fails because
`config.Load` requires `schemaVersion: 1`.

From the Magento project root (where `.magento.app.yaml` or `.platform.app.yaml` lives):

```sh
# Adobe Commerce Cloud → magelift.yaml (refuse if the path already exists)
magelift init --from-acc

# Upsun / Platform.sh
magelift init --from-upsun

# Overwrite an existing file only with explicit confirmation
magelift init --from-acc --yes

# Write a review copy without touching the default --config path
magelift init --from-acc --config-out review.magelift.yaml
```

`--from-acc` and `--from-upsun` are mutually exclusive. The mapper covers structural
app/services/routes/cron plus the frozen D-07 env allowlist (crypt → encryption
secret ref placeholder; `UPDATE_URLS`/routes → domain; DB/Redis/OpenSearch/AMQP
relationships → capability / catalog intent; `SCD_*` → `build.staticContent`). See
[ece-tools parity](ece-parity.md).

When any key is outside that allowlist, MageLift still writes the YAML, writes a
sidecar next to the output path (`magelift.unmapped.md`, or
`<stem>.unmapped.md` when using `--config-out`), and exits non-zero. Fix or accept
the residuals by hand before deploy.

Database dump / media cutover and brownfield attach remain later milestones (see
[post-beta roadmap](post-beta-roadmap.md)); Phase 4 only ships config onramp.

1. Run `magelift init --from-acc` or `--from-upsun`; review YAML + any unmapped sidecar.
2. Replace encryption / Composer secret placeholders with real Secrets Manager (or SSM) refs.
3. Add `preview`, `staging`, and `production` environments as needed.
4. Run `magelift doctor` and `magelift bootstrap` in a non-production AWS account.
5. Build and promote one digest; deploy preview with destroy-on-exit acceptance
   (`docs/aws-acceptance.md`) until destroy leaves zero tagged leftovers.
6. Point a real domain / ACM cert at the preview edge before promoting staging.

## Where to read next

- [Architecture](architecture.md) — product matrix and ports/adapters boundary
- [Configuration](configuration.md) — field reference for `magelift.yaml`
- [ece-tools parity](ece-parity.md) — closed vs intentional-gap matrix (D-06 / D-07)
- [Operations](operations.md) — logs, exec, CI generate, local `dev`
- [Local AWS acceptance](aws-acceptance.md) — free-tier-friendly real AWS loop
