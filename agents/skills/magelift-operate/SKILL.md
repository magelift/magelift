---
name: magelift-operate
description: >-
  Operate a MageLift environment after deployment. Use for status, health,
  logs, safe Magento commands, release promotion, rollback, and teardown.
version: 1.0.0
---

# Operate MageLift

Use this skill when the infrastructure already exists and you need to inspect
or change the selected environment.

## Working rules

- Select the environment explicitly with `--env` in automation.
- Run `magelift health --mode config` before a live operation.
- Use `magelift health --mode runtime` after deploy, rollback, or a provider change.
- GCP outputs, exec, logs, tunnels and health in outputs/runtime mode read the
  existing stack without a YAML image digest or local build journal. Deploy and preview still
  require the immutable artifact. Destroy does not require a build.
- With `nginx-fpm`, select `--container php-fpm` for PHP/Magento exec commands.
- First deployment initializes schema/data only when the database has zero tables.
  A migration failure stops without automatic retries. Preserve a partial database
  for diagnosis and repair; do not clear it to force initialization.
- Treat `outputs` as data. Do not paste secrets into tickets or agent prompts.
- Use `destroy --yes` only for the exact environment you intend to remove.
- `exec --service deploy` is rejected by design; read deploy logs instead.
  Expired previews refuse deploy but always allow destroy.
- Production deploys need `--ack-maintenance-drain` plus the incompatible-schema
  runbook in `docs/operations.md`; deploy success means the intended rollout
  plus a passing Magento probe, never scheduler settlement alone.
- If cleanup fails, keep the evidence and inspect resources by the MageLift run prefix.

## Useful commands

Laptop Magento path:

```sh
magelift --env staging doctor
magelift --env staging bootstrap
magelift --env staging deploy --yes
magelift --env staging health --mode runtime
magelift --env staging logs --service web
magelift --env staging exec --service web -- php -v
magelift --env staging cost --live
magelift --env staging cleanup plan --dir .magelift/cleanup
magelift --env staging destroy --yes
```

AWS bootstrap also needs `--access-log-bucket`. `cost --live` errors on GCP targets (live Catalog pricing is not wired there); omit `--live` on GCP. `sign` and `promote` use the
current cloud or CI login; omit Cosign flags.
For GCP Actions bootstrap, pass --github-owner and --github-repo. It reuses
GH_TOKEN, GITHUB_TOKEN, or gh auth login to resolve immutable repository IDs
before cloud creation, then creates separate build and deployment identities.
Review the returned ciPermissionPlan: deployment service roles apply to the
selected project, not only names with the MageLift prefix. Rerun with
--apply-ci-permissions to grant them, and --configure-github to write the
derived nonsecret repository variables. Existing different values are refused;
workflow and environment protections are preserved. Identity creation alone
does not prove registry permissions or a live OIDC exchange.
GCP bootstrap also prepares its registry, immutable remote state-key reference,
and an owned GKE node account with repository reader and the project-level
roles/container.defaultNodeServiceAccount role. This baseline runs without
GitHub options and needs project IAM management permission. New clusters use
the owned account; existing clusters keep their immutable node identity.
Use a fresh environment for migration. Environment destruction retains these
bootstrap resources; never modify the shared default Compute account.
Keep that secret while state or backups need it. Explicit Pulumi passphrase
environment variables remain available for existing stacks and recovery.

```sh
magelift sign --digest IMAGE@sha256:...
magelift --env staging promote --digest IMAGE@sha256:...
magelift --env staging rollback --ack-forward-only --to-sequence 2
```

## Report

Record the command, environment, image digest, result, and any resources left
after teardown. A green CLI exit code is not proof that an external DNS record
or manually created resource was removed.

For Cloud SQL creation ERROR_STOCKOUT, inspect the failed operation and resource
inventory before retrying. Preserve the state. New instances prefer the first
target.gcp.zones entry; select another compatible zone and reuse the same digest.
Existing instances keep their zone on CLI upgrades or reordered placement lists.
No automatic retry or deletion of an existing database is implied.
