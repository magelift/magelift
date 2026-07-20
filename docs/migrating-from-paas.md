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

## Suggested first week

1. Map ACC/Platform variables to Secrets Manager (or SSM) references.
2. Add `magelift.yaml` with `preview`, `staging`, and `production` environments.
3. Run `magelift doctor` and `magelift bootstrap` in a non-production AWS account.
4. Build and promote one digest; deploy preview with destroy-on-exit acceptance
   (`docs/aws-acceptance.md`) until destroy leaves zero tagged leftovers.
5. Point a real domain / ACM cert at the preview edge before promoting staging.

## Where to read next

- [Architecture](architecture.md) — product matrix and ports/adapters boundary
- [Configuration](configuration.md) — field reference for `magelift.yaml`
- [Operations](operations.md) — logs, exec, CI generate, local `dev`
- [Local AWS acceptance](aws-acceptance.md) — free-tier-friendly real AWS loop
