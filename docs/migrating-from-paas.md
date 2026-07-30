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

Phase 4 ships the config onramp above. Database dump import, media sync, and the
cutover runbook below ship in Phase 5. Brownfield attach of an existing VPC/DB
remains Phase 8 (see [post-beta roadmap](post-beta-roadmap.md)).

1. Run `magelift init --from-acc` or `--from-upsun`; review YAML + any unmapped sidecar.
2. Replace encryption / Composer secret placeholders with real Secrets Manager (or SSM) refs.
3. Add `preview`, `staging`, and `production` environments as needed.
4. Run `magelift doctor` and `magelift bootstrap` in a non-production AWS account.
5. Build and promote one digest; deploy preview with destroy-on-exit acceptance
   (`docs/aws-acceptance.md`) until destroy leaves zero tagged leftovers.
6. Follow the cutover runbook below before pointing production DNS at MageLift.

## Cutover runbook (dump, media, maintenance, reindex, verification, rollback)

This runbook moves a store onto a MageLift-managed environment. Phase 5 proves the
**executable local half** (dump import, media sync, maintenance/reindex/verify/rollback
command shapes) offline. **Live DNS cutover, a full non-prod rehearsal against a
managed cloud instance, and the managed-instance dump-import acceptance cell ride
Phase 7 under an explicit HUMAN_GATE** — no fourth paid AWS pass in Phase 5.

### Prerequisites

- Valid `magelift.yaml` with a non-production environment (preview/staging) and
  `seedDump` set via `magelift env create … --dump <path>` (or an existing overlay).
- Sanitized dump outside git; media tree available as a local directory.
- Deployed environment with stack outputs including `mediaBucket` (AWS) and a
  reachable DB for import.
- Day-2 path working: `magelift exec`, `magelift reindex`, `magelift logs`
  ([operations](operations.md)).

### 1. Import the database dump

First successful deploy with `seedDump` status `recorded` auto-imports once (hybrid
D-01). For explicit retries or dry runs of the same path:

```sh
# Journal: recorded|failed → importing → imported|failed
magelift env import-dump <environment>

# Retry into a non-empty database (MIGRATE-05 / D-04)
magelift --yes env import-dump <environment>

# Confirm journal
magelift env status <environment> -o json
```

Do not use `magelift dev seed` for cloud environments — that verb is local Compose only.

### 2. Sync media into object storage

```sh
# Merge upload: PutObject only; remote extras kept; fails if source keys missing after list
magelift env media-sync <environment> --source /path/to/media-or-pub
```

Key mapping: if `--source` contains `pub/media/`, keys are relative to that prefix;
otherwise `--source` is the media root. Offline evidence tier is unit + Floci
listing-diff (see [capability matrix](capability-matrix.md)); live bucket sync on a
paid account is not claimed by Phase 5.

### 3. Magento maintenance mode

Put Magento into maintenance before DNS or final traffic shift so shoppers do not
hit a half-cut store:

```sh
magelift exec --env <environment> --service web --container web -- \
  bin/magento maintenance:enable

# After verification (or on rollback):
magelift exec --env <environment> --service web --container web -- \
  bin/magento maintenance:disable
```

### 4. Reindex

```sh
magelift reindex --env <environment>
# equivalent fixed Magento path via ECS Exec:
# magelift exec --env <environment> --service web --container web -- bin/magento indexer:reindex
```

Optionally flush caches after reindex: `magelift cache-flush --env <environment>`.

### 5. Verification checks

Before DNS:

1. `magelift env status <environment> -o json` — `seedDumpStatus` is `imported` (or
   intentionally absent if no dump).
2. Storefront / admin health through the ALB or preview URL (`magelift` runtime
   health / browser smoke) — not only Pulumi success.
3. Sample SKUs, CMS pages, and a media URL from the synced bucket path.
4. `magelift logs --env <environment> --service web --since 30m` for fatal Magento
   errors; `magelift logs --service deploy` if a migrate candidate ran.

### 6. DNS cutover

Point the customer domain (and ACM certificate validation, if needed) at the MageLift
edge for that environment. DNS TTL, Route53/ACM wiring, and live non-prod rehearsal
evidence are **Phase 7 HUMAN_GATE** — document the planned records here operationally,
but do not treat Phase 5 docs or local scratch as SC5 live proof.

#### Cloudflare preview rehearsal (MIGRATE-04 / Phase 7)

Preferred non-prod host: `magelift-preview.alexandrecourtiol.com` (fallback
`magelift-preview.acourtiol.com`). Use an API token with **Zone.DNS Edit** (and
Zone.Zone Read) via `CLOUDFLARE_API_TOKEN` or `CF_API_TOKEN`. **Do not use Wrangler
OAuth** for DNS writes — Wrangler OAuth is zone:read only and cannot create/update
records.

```sh
export CLOUDFLARE_API_TOKEN=...   # Zone.DNS Edit — not Wrangler OAuth
export MAGELIFT_CUTOVER_HOST=magelift-preview.alexandrecourtiol.com
# Point at stack applicationURL / LB hostname or IPv4 from the paid pass:
TARGET=<ingress-or-lb> ./scripts/cutover-dns-cloudflare.sh
# Rehearse without mutating:
TARGET=example.invalid ./scripts/cutover-dns-cloudflare.sh --dry-run
# Harness EXIT / teardown:
./scripts/cutover-dns-cloudflare.sh --cleanup
```

Auth and zone notes: [Phase 7 Cloudflare handoff](../.planning/phases/06-shared-kubernetes-day-2/06-PHASE7-HANDOFF.md).

### 7. Rollback

If verification fails after traffic shift:

1. Revert DNS to the previous origin (lowest blast radius).
2. Disable Magento maintenance on the **previous** origin if you enabled it there;
   keep MageLift in maintenance until you decide to retry.
3. Infrastructure rollback for a disposable preview: `magelift env destroy <name> --yes`
   (protection must be off) or restore from `magelift` state backup/restore day-2
   verbs when the env is long-lived — see [operations](operations.md).
4. Re-import only with `--yes` when the target DB is non-empty; prefer a fresh preview
   env over overwriting production.

### Evidence honesty (MIGRATE-04 / SC5)

| Slice | Status |
| --- | --- |
| Runbook + local scratch (dump, media unit/Floci, maintenance/reindex/verify/rollback command shapes) | Phase 5 |
| Live DNS + full non-prod cutover rehearsal + managed-instance dump cell | Phase 7 HUMAN_GATE (no Phase 5 paid AWS pass) |

## Where to read next

- [Architecture](architecture.md) — product matrix and ports/adapters boundary
- [Configuration](configuration.md) — field reference for `magelift.yaml`
- [ece-tools parity](ece-parity.md) — closed vs intentional-gap matrix (D-06 / D-07)
- [Operations](operations.md) — logs, exec, CI generate, local `dev`
- [Local AWS acceptance](aws-acceptance.md) — free-tier-friendly real AWS loop
- [Capability matrix](capability-matrix.md) — evidence tiers for media / day-2 ports
