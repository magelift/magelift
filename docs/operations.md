# Deployment operations

The deployment contract is shared by every target. The provider-neutral orchestrator
defines validation, locking, preview, candidate registration, Magento migrations,
service updates, stabilization, health checks, and release recording. The AWS CLI
connects that workflow to an ECS candidate task, while Pulumi remains responsible for
durable services and infrastructure. Candidate task definitions are deregistered after
the migration phase, including when the phase fails. If a later hook or deployment
phase aborts, the orchestrator retries candidate cleanup with a short,
cancellation-safe timeout before releasing the deployment lock.

`magelift doctor` resolves the build configuration and every named environment. It
prints all readiness checks, including failures, before exiting. Exit code 4 means
the file parsed but one or more resolved contracts are invalid; read and schema
errors use the invalid-input exit code 2.

When `preview`, `deploy`, or `destroy` fails because another Pulumi update
holds the stack lock, the command exits 5 with a wait-and-retry sentence
instead of the generic failure, so CI can tell a collision from a graph bug.

Production requests need an approval and a digest-pinned signed image. Deployment
re-verifies the promoted digest against its recorded certificate identity and OIDC
issuer before any infrastructure mutation. A rollback is another forward deployment.
MageLift does not attempt to reverse database migrations, so rollback commands require
an explicit acknowledgement of that limitation.

## Incompatible schema changes (runbook)

Static content is baked into the image at build (the PHP lifecycle plan owns
it); deploy-time migration runs config import, schema upgrade, and cache
operations only, and serving containers receive baked assets by image-digest
identity. Deploy success means the intended rollout plus a passing bounded
Magento probe — scheduler settlement alone never reports success.

Old pods keep serving while the migration candidate runs, so incompatible
(destructive or irreversible) schema changes always ride maintenance mode.
Zero-downtime incompatible schema changes are explicitly not promised. For
any production deploy that may carry one:

1. Take a fresh backup (database plus media manifest) and confirm it restores.
2. Enable maintenance mode (`bin/magento maintenance:enable`).
3. Drain writers: stop cron and queue consumers; confirm no active writers.
4. Deploy with `--ack-maintenance-drain`, attesting steps 1–3.
5. Verify health output, then disable maintenance mode.
6. Scale consumers and cron back; watch the first scheduled runs.

A digest rollback never reverses a migration: rolled-back code on a migrated
schema is a forward fix (compatible follow-up migration), never a downgrade.
Rollback commands keep their forward-only acknowledgement for the same reason.

Preview environments must have a TTL and budget before infrastructure is created.
Protected environments reject destroy requests until protection is removed.

The v1 preview promise, per certified origin: database-backed queues and a
single-AZ failure domain on both; search disabled on AWS (opt in with
`searchMode` when the feature needs it); the 1-replica OpenSearch workload
on GCP. Previews stay cheap and short-lived by default; standard and
high-availability presets scale up from there.

`magelift env list` lists configured environments without contacting AWS. `magelift
env create <name>` adds a validated overlay using the supplied account, preset,
class, domain, TTL, budget, protection, and branch flags. `magelift env destroy
<name> --yes` destroys the selected infrastructure before removing its overlay.
`magelift
env protect <name> --on|--off` updates only that environment's protection flag using
an atomic file replacement. Disabling protection for a production class requires
`--yes`. `magelift status` reports the selected environment and its resolved target
settings.
`magelift env sweep --dry-run` reports expired preview overlays without contacting
AWS. A scheduled cleanup job can run `magelift env sweep --yes`; it destroys only
expired, unprotected environments with `class: preview`, then removes their
configuration overlays. Production and protected environments are always skipped.
MageLift ships the sweep command and the generated hourly workflow step; it does
not run a hosted expiry scheduler. The customer's scheduler (cron or CI) owns
the run, sweep authenticates with the operator's configured cloud credentials
(same as deploy and destroy), and if the scheduler skips or a sweep fails
partway the expired stacks keep billing until the next successful sweep or an
explicit destroy. After teardown, state backups (90-day retention), log and
metric retention, unattached addresses or volumes outside the stack, and
provider billing delay can all keep costing money; the ledger plus direct
provider inventory show what remains.
`magelift login` verifies that the current AWS credentials can access the selected
account and region. It does not write credentials or change AWS resources.
`magelift upgrade --check` checks the latest GitHub release without changing the
local binary. `magelift upgrade` downloads the matching OS/architecture archive,
verifies the keyless Sigstore bundle for `checksums.txt`, then verifies the
archive's SHA-256 entry before atomically replacing the current executable.
Release artifacts are built by GoReleaser and accompanied by GitHub artifact
attestations; package-manager installations remain an option for teams that do
not permit self-updates.
`magelift state status` inspects the bootstrap-created lock object and reports its
owner when another process holds it. `magelift state backup` takes a versioned
snapshot of Pulumi state while holding the deployment lock. Bootstrap retains
completed backup snapshots for 90 days and removes incomplete multipart uploads
after seven days. A backup becomes
restorable only after every state object has been copied and MageLift has written
its completion marker. `magelift state restore
<backup-id> --yes` removes state objects that are not in the snapshot and restores
the saved objects. `magelift state unlock --yes` is reserved for a stale lock and
reports the lock metadata it removed.

`magelift preview` validates the resolved AWS stack and prints the Pulumi change
summary. `magelift deploy` validates the immutable artifact, runs a fresh preview,
acquires the S3 deployment lock, executes a candidate task that imports `config.php`,
runs `setup:upgrade`, cleans and flushes Magento caches, applies the Pulumi update,
waits for ECS stabilization, and runs runtime health checks. Production
deployments require `--yes` and a matching release-journal entry with verified
signature metadata. CI can pass the exact build
output with `deploy --digest <registry>@sha256:<digest>` without changing project YAML.
`magelift destroy` previews before removing resources, and
`magelift outputs` reads the last successful stack outputs. Protected environments
must be unprotected explicitly before they can be destroyed. Magelift derives
the state backend from `magelift bootstrap` and `magelift.yaml`. Set
`PULUMI_BACKEND_URL` only as a CI override. The AWS
lock bucket is the deterministic bucket created by `magelift bootstrap`; bootstrap
must complete before deploy or destroy.

Compiled extensions may add deployment and post-deployment hooks through the Go SDK.
Hooks target stable operation IDs such as `deploy.migrations`, `deploy.update`, and
`post-deploy.health`, declare dependencies, and run with a timeout and bounded retry
policy. `replace` and `disable` are explicit graph operations; arbitrary numeric
priorities and shell-command matching are not supported. YAML remains limited to the
Magento preparation lifecycle, so ordinary projects do not need an extension.

`magelift secret set <name> --value-stdin` reads a value only from stdin and never
prints it. `magelift secret list` returns names and ARNs. `magelift secret remove
<name> --yes` schedules deletion with the normal 30-day recovery window.

`magelift build --push` and `magelift sign --digest <registry>@sha256:<digest>`
sign the published digest using the same login already used for Magento. On GCP
that is `gcloud`; MageLift impersonates the CI service account created by
`magelift bootstrap` (`ml-<project>-<env>-ci@<gcp-project>.iam.gserviceaccount.com`).
Override with `MAGELIFT_SIGNING_SERVICE_ACCOUNT` or
`CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT`. In GitHub Actions the workflow's
`id-token` permission is enough. Operators do not pass Cosign flags on the
YAML-only path.

`magelift promote --digest <registry>@sha256:<digest>` verifies that signature
with the current login and records the release. `--certificate-identity` and
`--certificate-oidc-issuer` remain available for CI overrides. Use `--from` to
name the environment that supplied the digest. Successful deployments append a
deploy event to the same journal. MageLift writes the release history to
`.magelift/releases/<environment>.jsonl` with an atomic file replacement; these
commands do not contact AWS.

`magelift history` reads that journal. `magelift evidence` exports digest, actor,
config provenance, and backup policy from that journal and the effective
configuration without secret values. `magelift audit` is a different report: it
lists encryption, IAM, logging, backup, WAF, and residency pointers from the
effective YAML. It does not certify SOC 2, ISO 27001, or GDPR. `magelift edge
purge` invalidates Fastly or the native CDN cache when the selected target
advertises purge; otherwise it fails with a typed unsupported error.

`magelift rollback --to-sequence <number>`
selects an earlier signed digest, verifies it again, refuses when the selected
digest cannot run on the live Magento schema, runs the same candidate-task and
service workflow with that digest, and records a new release that points to it.
Deploy journal rows often omit Cosign identity and schema epoch; rollback looks
those up from the newest promote of the same digest. Promote and deploy record
`schemaEpoch` from Magento `patch_list` plus `setup_module` counts only when a
dumpimport transport is explicit: `MAGELIFT_DUMPIMPORT_RUNNER=kube` with
namespace and pod/selector, or `MAGELIFT_DUMPIMPORT_HOST`. They do not dial
localhost MySQL. Missing epochs fail closed before cutover. It does not delete history or reverse
database migrations. Promote and rollback both
require `--yes` when the selected environment has `class: production`.

`magelift cost` is account-free by default and is routed through the selected
provider's `CostEstimator` adapter (same boundary as logs/ops; ADR 0004). AWS ECS
Fargate reports catalog capacity and budget inputs; `magelift cost --live` queries
the AWS Price List API for on-demand capacity in the selected region and separates
priced resources from capacity that remains an estimate and products AWS could not
match. GCP `cost --live` is not wired and returns an error telling the operator
to use account-free mode (omit `--live`). Experimental targets return
“not supported yet” until they ship an adapter.
Data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT processing
remain unsupported because they depend on real workload measurements. A budget is
never presented as a forecast. On GCP, `magelift cost --budget` reads only
budgets scoped exactly to the configured project and reports alert thresholds;
account-wide or multi-project budgets are omitted. The Cloud Billing Budget API
does not provide current or forecast spend through this read path, and a budget
does not block deployments. Every budget report carries `Enforced: false`:
MageLift validates the configured budget and reports alert thresholds; it never
caps or stops spend, and provider billing delay means recently destroyed
resources can still appear on the next invoice. Spend evidence needs the
provider's billing report or a configured billing export. The authenticated GCP
certification project has
the Budget API enabled, but its current principal lacks `billing.budgets.list`
on the attached billing account, so live budget proof remains blocked until a
billing-account reader is granted or another billing evidence source is wired.

For AWS, `magelift cost --budget` reads account-scoped cost budgets and their
percentage notifications through the read-only Budgets API. Preview environments
do not inherit those account or production limits; the report is scoped to the
preview environment and stays `not-configured` until a preview-owned budget
exists. Expensive preview catalog choices (Amazon MQ, provisioned OpenSearch,
multi-instance Aurora) are flagged in the account-free notice. It includes the
provider's current and forecast values when returned, but account budgets are
not treated as MageLift environment-owned or deployment-enforcing unless a
future ownership contract proves that mapping. OVHcloud and Scaleway report
budget state as unavailable until their authenticated budget APIs and scope
semantics are verified; `monthlyBudgetCents` remains a planning input only.

Standard and high-availability presets include private interface endpoints for the
runtime's AWS control-plane traffic. Preview uses the S3 gateway endpoint only; its
remaining AWS API and image/log traffic uses the preview NAT path to avoid adding
fixed endpoint-hour charges to disposable environments.

An existing VPC can be supplied with explicit public, private, and data subnet IDs.
MageLift then leaves routing, NAT, and endpoint ownership with that network and
validates the imported subnet shape before Pulumi resource registration.

`magelift health` defaults to account-free configuration checks. `--mode outputs`
checks that the last successful stack exposes structurally valid HTTPS application
and media URLs; reading outputs may require access to the configured Pulumi backend.
`--mode runtime` reads provider runtime state (ECS service/task or Kubernetes
rollout) and does not infer health from configuration or stack outputs. Each check
includes a `layer` of `infrastructure`, `service`, `magento`, `dependency`, or
`deployment`. Magento down with a healthy database is overall unhealthy (exit 4).
Exit code 4 means a check failed, while exit code 3 means the requested evidence is
unavailable. Production AWS stacks additionally run a CloudWatch Synthetics check
against the public
`/health` endpoint every five minutes. The canary's `SuccessPercent` alarm treats
missing data as a failure and sends notifications through the configured SNS topic.
Run artifacts are encrypted, versioned, and lifecycle-managed in the private
synthetic-artifact bucket. The CLI health command reports ECS evidence; canary run
history and screenshots remain in CloudWatch Synthetics.

`magelift ci generate --magelift-version vX.Y.Z` writes a deterministic GitHub Actions
workflow for the configured provider/runtime and environment matrix. The shared
workflow validates every environment, builds and signs one digest, previews opt-in
pull requests labeled `magelift-preview`, destroys a labeled preview when its pull
request closes, deploys `staging` on main, sweeps expired previews hourly, and gates
a manual `production` run with the GitHub production environment. Fixed environment
names are unchanged; preview names derive from the repository and pull-request
number, while branch and commit values remain diagnostic metadata.

The AWS generator uses the role variables returned by `magelift bootstrap` with
GitHub flags. The GCP generator uses `MAGELIFT_GCP_PROJECT_ID`,
`MAGELIFT_GCP_REGION`, `MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER`, and
`MAGELIFT_GCP_SERVICE_ACCOUNT` for GitHub federation. Magelift derives stack
state from `magelift.yaml` after bootstrap; `PULUMI_BACKEND_URL` is only a CI
override. Image references remain required for `build --push`. Promote uses the
workflow's GitHub login. GCP uses short-lived credentials;
long-lived service-account keys are not part of the workflow contract.

`magelift ci validate` detects a missing, edited, or configuration-drifted workflow
without contacting a cloud provider. Preview apply and close jobs are serialized by
repository and pull-request, with cancellation disabled. Ownership and generation
checks still run before provider mutation because queued GitHub events can be stale.

`magelift logs --service web|deploy|cron` reads the selected ECS service's structured
CloudWatch log group. Use `--since 15m` or an RFC3339 timestamp, `--filter` for a
CloudWatch Logs pattern, and `--limit` to bound the response. The command paginates
and sorts events before writing table, JSON, or YAML output; it does not fall back to
local container logs when AWS evidence is unavailable.

`magelift exec --service web --container web -- <command>` selects a running task and
starts the AWS-supported ECS Exec session through the locally installed AWS CLI.
`cache-flush`, `reindex`, `cron-run`, and `queue-status` use the same path with a
fixed Magento command. `magelift ssh` is a compatibility name for an ECS Exec
shell. The runtime has no inbound SSH rule. The command runner never interpolates user
arguments into a local shell, although the command string is interpreted by the
selected container as required by ECS Exec.
`--service deploy` is rejected: migrate is a one-off candidate task, not a durable
service. Use `magelift logs --service deploy` for migration output.
`magelift tunnel` is capability-driven. Kubernetes runtimes can forward the
application, queue, queue management UI, and search API through `kubectl`; the
database path is intentionally unsupported there until a provider-native private
proxy is available. GCP database tunnels use the Cloud SQL Auth Proxy and require
the private connection output. Database and dashboard paths that are not verified
by the selected adapter fail explicitly; MageLift never falls back to a public
endpoint.

Local development stays account-free. `magelift local init` resolves the
source-dated local compatibility row for the configured Magento release before it
writes `.magelift/compose.local.yml`. The plan includes the PHP branch, Composer
requirement, requested extensions, PHP settings, email mode, pinned service images,
health probes, and loopback ports. A release or service family without a verified
local contract fails before Compose creates a volume. Use the project-level
`local` block for deliberate choices, for example:

```yaml
local:
  database: {family: mysql, version: "8.4"}
  queue: {family: rabbitmq, version: "4.2"}
  webServer: {family: nginx, version: "1.30"}
  webCache: {family: varnish, version: "8"}
  phpSettings: {memory_limit: 1G, max_execution_time: "180"}
  email: {mode: smtp, host: mail.example.test, port: 2525, from: shop@example.test}
```

`local init` also writes `.magelift/local.php.ini` and mounts it as the final PHP
configuration layer in the app container. The nginx + PHP-FPM runtime image
builds the verified local extension baseline; an extension outside that baseline
fails during planning instead of being reported as available metadata.

Then use `magelift local up --service database` for the database, or
`magelift local up --service app` for the full Magento stack. The app profile
runs the selected web-runtime image, which is nginx + PHP-FPM by default, plus OpenSearch and
RabbitMQ. `frankenphp-classic` and `php-apache` require `compatibility.allowUnsupported`; they have no Adobe row and are not certified. `frankenphp-worker` is unregistered. Selecting `webCache: varnish`
adds Varnish in front of nginx and keeps the app's direct HTTP port on 8081 by
default. `magelift local status`, `magelift local logs`, and
`magelift local exec --service app -- <command>` complete the workflow.
`magelift local reset --yes` removes all local capability volumes. Generated service
images are digest-pinned and can be reviewed or deliberately changed in the
generated Compose file. Artemis uses Magento's STOMP queue transport and a
Jolokia-backed broker image contract. Redis remains outside the current Adobe
rows and requires `compatibility.allowUnsupported: true`; MageLift reports that
choice as an explicit warning. SMTP and SES modes write Magento's
`system.smtp` configuration. Both take explicit host, port, username, and a
credential environment variable; no vendor endpoints are baked in. Point
`smtp` at any provider relay: OVH mailbox (`smtp.mail.ovh.net`, port 465
SSL/TLS or 587 STARTTLS, full mailbox address as username, about 200 mails
per hour and not for bulk), Scaleway TEM (`smtp.tem.scaleway.com`, port 587
STARTTLS or 465/2465 TLS, Project ID as username, API secret key as password),
or Cloudflare Email Sending (`smtp.mx.cloudflare.net`, port 465 implicit TLS
only, username `api_token`, API token with Email Sending permission as
password; beta), or SendGrid (`smtp.sendgrid.net`, port 587, username
`apikey`, API key as password; manual-only until a stable Pulumi package
exists). GCP and Fastly have no native email sending; shops there
point `smtp` at SES or another relay.
Put the credential variable in `.magelift/local.env` when the app needs it. The
resolver reads it inside the container without writing the secret to the
generated Compose file. `local.email.mode: mailpit` starts a digest-pinned
Mailpit sidecar (`axllent/mailpit:v1.30.7`) on loopback SMTP 1025 / UI 8025
with `/mailpit readyz`, and wires Magento SMTP to that sink. It is not cloud
delivery. The generated credentials and
loopback-only ports are for local development only; do not reuse them in a
deployed environment. The app also listens on `https://localhost:8443`. Use
`MAGELIFT_LOCAL_HTTPS_PORT` to change the host port. The certificate is
local and ephemeral; trust it only in a development profile, and do not use it
as production TLS evidence.

For a Magento repository with dependencies already installed, run
`MAGELIFT_LOCAL_ADMIN_PASSWORD='...' magelift local seed`. The command starts the
app profile, runs `bin/magento setup:install` with local capability defaults, and
writes the administrator password only to `.magelift/local.env` with mode 0600.
The generated `.magelift/` directory is ignored by Git. If `app/etc/env.php`
already exists, seeding reports that the project is installed and does not run a
second installation. The password must be at least 16 alphanumeric characters;
the command never prints it or places it in the Docker command arguments.

## Operator verbs

Five verbs cover run, debug, and spend on certified origins, live-proved
together on GCP (`mldp8`, 2026-09-15) with AWS creds-only cost reads the
same day. `health --mode runtime` reports layered checks with observed
sources and refuses to infer. `logs --service web|cron` streams the
workload logs. `exec --service web|cron -- <command>` runs inside the
workload; `--service deploy` is rejected by design with a pointer to the
deploy logs. `cost` prints capacity inputs and an explicit unpriced list
in account-free mode; `cost --live` on AWS returns current on-demand
prices for Fargate, Valkey, and the provisioned data services, and
`cost --budget` reads account budgets with thresholds while previews
never inherit them. `cleanup claim|record|plan|reconcile` closes the
loop on interrupted runs: claim and record build a ledger, plan shows
what a reconcile would delete, and reconcile with `--yes` deletes only
ledger-owned live resources in rank order. Expired previews refuse
deploy and always allow destroy, on every provider.

## Magento YAML overlays

`application.magento` writes Magento `env.php` / `CONFIG__*` overlays. It does
not create Magento websites, stores, or store views. Those stay in Magento.

```yaml
application:
  magento:
    frontName: admin
    cookieDomain: ".shop.example"
    corsOrigins: ["https://storefront.example"]
    consumers:
      mode: processes
      names: [product_action_attribute.update]
```

`frontName` is also the AWS WAF admin path. Extra Magento consumers are worker
config, not Adobe B2B installation.

SQS and Pub/Sub are Magento-module transports, not `catalog.queueMode`. Set
`application.magento.queueTransport` (`sqs` or `pubsub`) and
`application.magento.queueModule` to a package that exists in `composer.lock`.
MageLift does not provision those brokers as catalog cells.

Optional `resilience.dataRegion` fails closed when the selected region or GCP
backup location sits outside that residency (for example `eu` vs a US Cloud SQL
backup location). AWS standard and high-availability split Valkey cache and
session endpoints; preview, GCP, OVH, and Scaleway share one cache endpoint for
both.

Brownfield attach is AWS VPC + RDS MySQL only. MageLift will not destroy an
adopted VPC or database. There is no Cloud SQL, OVH, or Scaleway database
adopt path.

Deployed tasks use a non-secret `env.php` scaffold from the runtime image. ECS
injects the configured Magento encryption key and managed database JSON fields
through Secrets Manager references, then exposes Adobe's `MAGENTO_DC_*` environment
configuration before the application starts. Configure
`target.aws.encryptionKeySecretArn` with a stable secret and never commit its value.

OpenSearch has two Magento contracts. Provisioned domains use HTTPS to the
domain on 443 inside the VPC, HTTP auth off, fine-grained access off, no
signing sidecar. OpenSearch Serverless (AOSS) still requires SigV4, so Magento
talks to a loopback aws-sigv4-proxy sidecar. Unit mocks cover both graphs.
Live Magento search (index/query/reconnect) remains a paid acceptance
checklist; Floci does not prove that data-plane. A private Magento-on-AWS shop
may illustrate the provisioned unsigned path
([sources/prior-terraform-opensearch.md](sources/prior-terraform-opensearch.md)); it is not
certification evidence and not the only supported AWS shape. Aurora Magento
env uses the writer endpoint plus Secrets Manager JSON, same as RDS. The
Adobe-gated pin is Aurora 3.11/3.12; unaimed 8.4 fails closed.

Use `make floci-test-aws` for account-free bootstrap, state, lock, versioned-media
restore, ECS runtime health, ephemeral candidate registration, Secrets Manager, and
CloudWatch Logs tests against digest-pinned Floci AWS 1.7.0. Use
`make floci-gcp-test` for GCS, Secret Manager, Pub/Sub, Logging, and Monitoring contracts against digest-pinned floci-gcp 0.7.0.
Emulators do not replace a real-cloud pass: GitHub Actions OIDC token exchange,
Autopilot, Memorystore, Armor, managed TLS, and Magento Cloud SQL PITR still need
packed live sessions ([certification sessions](certification-sessions.md)).
Floci AWS 1.7.0 does close IAM Create/Get/Tag OpenID Connect Provider
(`TestIAMOpenIDConnectProviderAgainstFloci`).
See [Local AWS acceptance](aws-acceptance.md). The composed Pulumi target is tested
separately with provider mocks, including preview, standard, and high-availability
resource graphs.

## Store loop runbooks (alpha)

Proved end to end on the [alpha recipe](alpha-recipe.md) during
reference-store acceptance. Each runbook below lists the exact commands
executed, in order; shells marked unproved are filled when the loop runs.

### Deploy the recipe (unproved)

<!-- Filled from the executed loop in reference-store-acceptance 2.1. -->

### Recover a failed release (unproved)

<!-- Filled from the executed loop in reference-store-acceptance 2.2. -->

### Back up and restore (unproved)

<!-- Filled from the executed loop in reference-store-acceptance 2.2. -->

### Expire a preview (unproved)

<!-- Filled from the executed loop in reference-store-acceptance 2.3. -->
