# Deployment operations

MageLift validates the environment, takes its lock and previews the Pulumi
changes. The provider registers the candidate, runs Magento migrations and
updates the service. MageLift checks the rollout and the running shop before
recording the release. On ECS, the provider runs migrations in a candidate task
and deregisters its task definition afterwards. GKE and AKS run Magento
workloads in Kubernetes. If the deployment stops early, MageLift gives cleanup
time to finish before releasing the lock.

`magelift doctor` resolves the build configuration and every named environment. It
prints every readiness check before exiting. Exit code 4 means the file parsed
and a resolved setting failed a check. File read and schema errors exit 2.

Use `magelift doctor --local` before cloud login or deployment. It checks local
tools and configuration without probing cloud credentials or downloading a
provider. The command reports missing or invalid configuration alongside the
other checks. Local readiness failures exit 4. Installing tools requires the separate
`--install-dependencies` option and its normal confirmation.

For a local support report, run:

```sh
magelift doctor --local --export doctor-metadata.json
```

MageLift creates the file locally with mode 0600 and refuses an existing file
or symlink. The export contains the recognized CLI version, operating system,
architecture and target category. It includes check categories, statuses and
counts. It reports whether a recognized tool is required or optional and
separates missing optional tools from failed checks.

The export omits names, account IDs, paths, configuration values, free-text
errors, tool output, tokens, URLs and logs. Unrecognized version strings and
target categories are also omitted. The console report may include private
diagnostics. Review raw logs before sharing them because extensions can write
private data. The export works with a missing project file. Without
`--local`, invalid-input failures still exit 2.

`preview`, `deploy`, and `destroy` exit 5 when another Pulumi update
holds the stack lock. Wait for the active update to finish before retrying.

If CI supplied the image digest during deployment, you can still run
`magelift edge purge --env staging` from a fresh checkout. On ECS with CloudFront
or GKE with Cloud CDN, the provider reads the deployed cache identity and checks
that it belongs to the storefront. GKE also uses existing stack outputs for
exec, logs, tunnels and runtime health checks. Supply an immutable image for
deploy and preview. Destroy checks environment protection and resource ownership.
[GKE runtime operations](adr/0019-gcp-runtime-operation-plans.md)

For a database with zero tables, the deployment migration first runs Magento's
schema and data upgrade commands, then the normal upgrade, configuration import
and cache commands. An existing database skips this initial step. This does not
create an administrator. A failed migration stops deployment without an automatic
Job retry. Inspect the failing step and database first. MageLift leaves a
partially initialized database in place so you can diagnose and repair it.
See [ADR 0021](adr/0021-empty-magento-database-bootstrap.md).

Magento readiness checks the database, configured search engine and endpoint.
Bare OpenSearch service names use HTTP on port 9200, matching runtime settings.
Explicit HTTP(S) endpoints keep their configured port and path. SigV4-only
endpoints skip the unsigned reachability request. See
[ADR 0012](adr/0012-search-proof-shapes.md).

Production requests need an approval and a digest-pinned signed image. Deployment
re-verifies the promoted digest against its recorded certificate identity and OIDC
issuer before any infrastructure mutation. A rollback is another forward deployment.
MageLift does not attempt to reverse database migrations, so rollback commands require
an explicit acknowledgement of that limitation.

## Incompatible schema changes (runbook)

The build packages static assets into the application image. During rollout,
MageLift upgrades the database, imports configuration and runs cache commands.
It then checks that the serving endpoint responds and that Magento is ready.

Schedule a maintenance window for a destructive or irreversible production
schema change. Your team manages maintenance mode on the serving replicas and
stops background writers. The acknowledgement flag records that an operator
completed that work.

1. Take a fresh backup (database plus media export) and confirm it restores.
2. Enable maintenance mode (`bin/magento maintenance:enable`).
3. Stop cron and queue consumers. Confirm that no writer is active.
4. Deploy with `--ack-maintenance-drain`, attesting steps 1-3.
5. Verify health output, then disable maintenance mode.
6. Scale consumers and cron back up, then watch the first scheduled runs.

A digest rollback never reverses a migration: rolled-back code on a migrated
schema is a forward fix (compatible follow-up migration), never a downgrade.
Rollback commands keep their forward-only acknowledgement for the same reason.

Preview environments must have a TTL and budget before infrastructure is created.
Protected environments reject destroy requests until protection is removed.

The PHP starter keeps the application and service families consistent across
stable and preview environments. Pull requests use a smaller, time-limited
topology and database-backed messaging by default. Managed databases and
search services can still incur substantial per-environment charges.

`magelift env list` reads the environments in your PHP project locally. `magelift
env create <name>` adds a validated overlay using the supplied account, preset,
class, domain, TTL, budget, protection, and branch flags. `magelift env destroy
<name> --yes` destroys the selected infrastructure before removing its overlay.
`magelift
env protect <name> --on|--off` updates only that environment's protection flag using
an atomic file replacement. Disabling protection for a production class requires
`--yes`. `magelift status` reports the selected environment and its resolved target
settings.

`magelift env sweep --dry-run` shows which preview environments have expired.
It reads their local configuration. The generated hourly workflow runs
`magelift env sweep --yes` with the shop’s cloud credentials. It destroys
expired, unprotected previews and removes each overlay after teardown succeeds.
If your scheduler misses a run or teardown fails, those resources keep accruing
charges. Check the deployment ledger and the provider inventory for retained
backups, logs, addresses and volumes.

`magelift login --env staging` asks the configured provider to check the current
credentials against the environment's cloud account.
`magelift upgrade --check` checks the latest GitHub release without changing the
local binary. `magelift upgrade` downloads the matching OS/architecture archive,
verifies the keyless Sigstore bundle for `checksums.txt`, then verifies the
archive's SHA-256 entry before atomically replacing the current executable.
GoReleaser builds the release archives, and GitHub publishes artifact
attestations. Teams that restrict self-updates can install with a package
manager.
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

`magelift preview --env staging` shows the Pulumi changes for that environment.
`magelift deploy` checks the image digest, takes the environment lock, runs
Magento's deployment steps and checks the running application. Production requires
`--yes` and a verified signature recorded in the release journal. CI can pass
the digest directly with `deploy --digest <registry>@sha256:<digest>`.

Run `magelift bootstrap` before the first deploy. AWS keeps the state lock in
S3, GCP in Cloud Storage and Azure in Blob Storage. `magelift outputs` reads the
last stack outputs. `magelift destroy` previews the removal and checks environment
protection. [Bootstrap the selected provider](bootstrap.md)

Compiled extensions may add deployment and post-deployment hooks through the Go SDK.
Hooks target stable operation IDs such as `deploy.migrations`, `deploy.update`, and
`post-deploy.health`, declare dependencies, and run with a timeout and bounded retry
policy. Use `replace` and `disable` to change an explicit operation. Hooks
match operation IDs. Project configuration can customize the Magento build
lifecycle without compiling an extension.

`magelift secret set <name> --value-stdin` reads the value from standard input.
`magelift secret list` displays the provider's secret references. On AWS,
`magelift secret remove <name> --yes` schedules deletion with a 30-day recovery
window. On other clouds, recovery follows the destination secret store's policy.

`magelift build --push` and `magelift sign --digest <registry>@sha256:<digest>`
sign the published digest using your active login. On GCP, MageLift uses
`gcloud` to impersonate the CI service account created by
`magelift bootstrap` (`ml-<project>-<env>-ci@<gcp-project>.iam.gserviceaccount.com`).
Override with `MAGELIFT_SIGNING_SERVICE_ACCOUNT` or
`CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT`. In GitHub Actions the workflow's
`id-token` permission is enough. Operators do not pass Cosign flags on the
PHP project.

`magelift promote --digest <registry>@sha256:<digest>` verifies that signature
with the current login and records the release. `--certificate-identity` and
`--certificate-oidc-issuer` remain available for CI overrides. Use `--from` to
name the environment that supplied the digest. Successful deployments append a
deploy event to the same journal. MageLift writes the release history to
`.magelift/releases/<environment>.jsonl` with an atomic file replacement. The
journal is local and the release recording needs no AWS API calls.

`magelift history` reads the release journal. `magelift evidence` exports the
image digest, actor, configuration source and backup policy with secret values
omitted. `magelift audit` lists the encryption, IAM, logging, backup, WAF and
residency settings that an auditor can inspect.

`magelift edge purge --env staging` clears the storefront cache on a configured
Fastly service, an ECS CloudFront distribution or GKE Cloud CDN. CloudFront uses
the distribution ID in the stack outputs and checks its ownership marker. On GKE,
MageLift reads the serving URL map reported by the shop's Ingress and checks the
storefront hostname. A load balancer address alone is insufficient to identify
the cache. [GKE edge outputs](bootstrap.md#gcp-native-edge-dns)

`magelift rollback --to-sequence <number>`
selects an earlier signed digest, verifies it again, refuses when the selected
digest cannot run on the live Magento schema, runs the same candidate-task and
service workflow with that digest, and records a new release that points to it.
When a deploy event omits Cosign identity or schema epoch, rollback reads
them from the newest promotion of the same digest. Promote and deploy record
`schemaEpoch` from Magento `patch_list` plus `setup_module` counts only when a
dumpimport transport is explicit: `MAGELIFT_DUMPIMPORT_RUNNER=kube` with
namespace and pod/selector, or `MAGELIFT_DUMPIMPORT_HOST`. They do not dial
localhost MySQL. Missing epochs fail closed before cutover. It does not delete history or reverse
database migrations. Promote and rollback both
require `--yes` when the selected environment has `class: production`.

`magelift cost` uses the selected provider's `CostEstimator` adapter. The
default estimate needs no account credentials. AWS ECS Fargate reports
catalog capacity and budget inputs. `magelift cost --live` queries
the AWS Price List API for on-demand capacity in the selected region and separates
priced resources from capacity that remains an estimate and products AWS could not
match. GCP `cost --live` is not wired and returns an error telling the operator
to use account-free mode (omit `--live`). Experimental targets return
“not supported yet” until they ship an adapter.
Data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT processing
remain unsupported because they depend on real workload measurements. A budget is
never presented as a forecast. On GCP, `magelift cost --budget` reads only
budgets scoped to that Google Cloud project alone and reports alert thresholds.
The Cloud Billing Budget API
does not return current or forecast spend through this read path. Budget reports
carry `Enforced: false` because the thresholds are alerts. MageLift validates and
reports them while deployments proceed as usual. Provider billing delay means
recently destroyed resources can appear on the next invoice. For actual spend, use the
provider's billing report or your billing export. The GCP budget reader needs
`billing.budgets.list` on the associated billing account.

For AWS, `magelift cost --budget` reads account-scoped cost budgets and their
percentage notifications through the read-only Budgets API. The preview report
stays `not-configured` until that preview has its own budget. Expensive preview
catalog choices (Amazon MQ, provisioned OpenSearch,
multi-instance Aurora) are flagged in the account-free notice. It includes the
provider's current and forecast values when returned, but account budgets are
not treated as MageLift environment-owned or deployment-enforcing unless a
provider report identifies the environment. Treat `monthlyBudgetCents` as a
planning input.

Standard and high-availability presets include private interface endpoints for the
runtime's AWS control-plane traffic. The preview has an S3 gateway endpoint.
Its other AWS API, image and log traffic uses NAT to avoid hourly interface
endpoint charges.

An existing VPC can be supplied with explicit public, private, and data subnet IDs.
MageLift then leaves routing, NAT, and endpoint ownership with that network and
validates the imported subnet shape before Pulumi resource registration.

`magelift health` defaults to account-free configuration checks. `--mode outputs`
checks that the last successful stack exposes structurally valid HTTPS application
and media URLs. The command may need access to the configured Pulumi backend.
`--mode runtime` reads provider runtime state (ECS service/task or Kubernetes
rollout) and does not infer health from configuration or stack outputs. Each check
includes a `layer` of `infrastructure`, `service`, `magento`, `dependency`, or
`deployment`. Magento down with a healthy database is overall unhealthy (exit 4).
Exit code 4 means a check failed, while exit code 3 means the requested evidence is
unavailable. Production AWS stacks run a CloudWatch Synthetics check
against the public
`/health` endpoint every five minutes. The canary's `SuccessPercent` alarm treats
missing data as a failure and sends notifications through the configured SNS topic.
Run artifacts are encrypted, versioned, and lifecycle-managed in the private
synthetic-artifact bucket. The CLI health command reports ECS checks. Find
canary history and screenshots in CloudWatch Synthetics.

`magelift ci generate --magelift-version vX.Y.Z` writes a deterministic GitHub Actions
workflow for the configured provider/runtime and environment matrix. The shared
workflow validates every environment, builds and signs one digest, previews opt-in
pull requests labeled `magelift-preview`, destroys the preview when its label is removed or its pull
request closes, deploys `staging` on main, sweeps expired previews hourly, and gates
a manual `production` run with the GitHub production environment. Stable
environments keep their names. Preview names use the repository and pull-request
number. The branch and commit appear in diagnostics.

The AWS generator uses the role variables returned by `magelift bootstrap` with
GitHub flags. The GCP generator uses `MAGELIFT_GCP_PROJECT_ID`,
`MAGELIFT_GCP_REGION`, `MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER`, and
separate build and environment service-account variables for GitHub federation
(see [bootstrap](bootstrap.md#generated-ci-and-pull-request-previews)). MageLift
derives stack state from `magelift.php` after bootstrap. `build --push` needs
only the final application image destination. Promotion consumes an existing
build run and signed digest. It verifies the identity and issuer of the build
job. Deployment uses a separate login. GCP exchanges the job identity for
short-lived credentials.

`magelift ci validate` detects a missing, edited, or configuration-drifted workflow
without contacting a cloud provider. Preview apply and close jobs are serialized by
repository and pull-request, with cancellation disabled. Ownership and generation
checks still run before provider mutation because queued GitHub events can be stale.

`magelift logs --service web|deploy|cron` reads the selected ECS service's structured
CloudWatch log group. Use `--since 15m` or an RFC3339 timestamp, `--filter` for a
CloudWatch Logs pattern, and `--limit` to bound the response. The command paginates
and sorts events before writing table, JSON, or YAML output. If AWS logs are
unavailable, the command reports the failure.

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
application, queue, queue management UI and search API through `kubectl`.
Database tunnels need a provider-native private proxy. GCP uses the Cloud SQL
Auth Proxy and requires the private connection output. The command reports
unsupported database and dashboard paths instead of opening a public endpoint.

Local development stays account-free. `magelift local init` resolves the
source-dated local compatibility row for the configured Magento release before it
writes `.magelift/compose.local.yml`. The plan includes the PHP branch, Composer
requirement, requested extensions, PHP settings, email mode, pinned service images,
health probes, and loopback ports. A release or service family without a verified
local contract fails before Compose creates a volume. Use the project-level
`local` block for deliberate choices, for example:

```php
[
    'local' => [
        'database' => [
            'family' => 'mysql',
            'version' => '8.4',
        ],
        'queue' => [
            'family' => 'rabbitmq',
            'version' => '4.3',
        ],
        'webServer' => [
            'family' => 'nginx',
            'version' => '1.30',
        ],
        'webCache' => [
            'family' => 'varnish',
            'version' => '8',
        ],
        'phpSettings' => [
            'memory_limit' => '1G',
            'max_execution_time' => '180',
        ],
        'email' => [
            'mode' => 'smtp',
            'host' => 'mail.example.test',
            'port' => 2525,
            'from' => 'shop@example.test',
        ],
    ],
]
```

`local init` also writes `.magelift/local.php.ini` and mounts it as the final PHP
configuration layer in the app container. The nginx/PHP-FPM recipe includes a fixed list of extensions. Planning reports
a missing extension before Docker starts.

Then use `magelift local up --service database` for the database, or
`magelift local up --service app` for the full Magento stack. The app profile
runs nginx and PHP-FPM with the selected services. Setting `webCache: varnish`
adds Varnish in front of nginx and keeps the app's direct HTTP port on 8081 by
default. `magelift local status`, `magelift local logs`, and
`magelift local exec --service app -- <command>` complete the workflow.
`magelift local reset --yes` removes all local capability volumes. Service containers use upstream tags derived from the selection. A project may supply an explicit image digest. Artemis uses Magento's STOMP queue transport and a
Jolokia-backed broker image contract. MageLift warns about Redis when it falls
outside the Adobe version guidance for the selected release. SMTP and SES modes write Magento's
`system.smtp` configuration. Both take explicit host, port, username, and a
credential environment variable. Use the host and port provided by your SMTP relay.
Put the credential variable in `.magelift/local.env` when the app needs it. The
resolver reads it inside the container without writing the secret to the
generated Compose file. `local.email.mode: mailpit` starts a digest-pinned
Mailpit sidecar (`axllent/mailpit:v1.33.0`) on loopback SMTP 1025 / UI 8025
with `/mailpit readyz` and points Magento at that SMTP server. Keep local
credentials on the workstation. The app also listens on `https://localhost:8443`
with an ephemeral development certificate. Set `MAGELIFT_LOCAL_HTTPS_PORT`
to change the local HTTPS port.

For a Magento repository with dependencies already installed, run
`MAGELIFT_LOCAL_ADMIN_PASSWORD='...' magelift local seed`. The command starts the
app profile, runs `bin/magento setup:install` with local capability defaults, and
writes the administrator password only to `.magelift/local.env` with mode 0600.
The generated `.magelift/` directory is ignored by Git. If `app/etc/env.php`
already exists, seeding reports that the project is installed and does not run a
second installation. The password must be at least 16 alphanumeric characters;
MageLift keeps it out of output and Docker command arguments.

## Operator verbs

Five verbs cover operation, debugging and spending on AWS and GCP. A historical run exercised them together on GCP (`mldp8`, 2026-09-15) with AWS creds-only cost reads the
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

## Magento application settings

`application.magento` writes Magento `env.php` / `CONFIG__*` overlays. It does
not create Magento websites, stores, or store views. Those stay in Magento.

```php
[
    'application' => [
        'magento' => [
            'frontName' => 'admin',
            'cookieDomain' => '.shop.example',
            'corsOrigins' => ['https://storefront.example'],
            'consumers' => [
                'mode' => 'processes',
                'names' => ['product_action_attribute.update'],
            ],
        ],
    ],
]
```

`frontName` is also the AWS WAF admin path. Extra Magento consumers are worker
config, not Adobe B2B installation.

SQS and Pub/Sub are Magento-module transports, not `catalog.queueMode`. Set
`application.magento.queueTransport` (`sqs` or `pubsub`) and
`application.magento.queueModule` to a package that exists in `composer.lock`.
MageLift does not provision those brokers as catalog cells.

Optional `resilience.dataRegion` fails closed when the selected region or GCP
backup location sits outside that residency (for example `eu` vs a US Cloud SQL
backup location). AWS standard and high-availability presets split the cache
and session stores. GKE and AKS can also run separate stores. Give sessions
their own memory budget and eviction policy when the shop needs customers to
stay signed in during a cache flush. [Configure cache and sessions](service-bindings.md#keep-sessions-when-cache-entries-expire)

To keep an existing database or broker on AWS, GCP or Azure, set its service
ownership to `existing` and supply a reachable endpoint and credential reference.
The attachment stays under its original owner's control when MageLift removes
the environment. Each runtime supports different roles. [Supported attachments](service-bindings.md#connect-to-existing-services)

ECS tasks use a non-secret `env.php` scaffold from the runtime image. ECS
injects the configured Magento encryption key and managed database JSON fields
through Secrets Manager references, then exposes Adobe's `MAGENTO_DC_*` environment
configuration before the application starts. Configure
`target.aws.encryptionKeySecretArn` with a stable secret and keep its value in the secret store. RDS and Aurora use the writer endpoint and read the database credentials from Secrets Manager.

OpenSearch has two Magento contracts. Provisioned domains use HTTPS to the
domain on 443 inside the VPC, HTTP auth off, fine-grained access off, no
signing sidecar. OpenSearch Serverless (AOSS) still requires SigV4, so Magento
talks to a loopback aws-sigv4-proxy sidecar. Pulumi mocks cover both graphs.
For a staging rehearsal, reindex a representative catalog, run storefront
queries and check reconnection after a node restart. An ElasticSuite shop also
needs the ICU and phonetic analyzers. [ElasticSuite on OpenSearch](service-bindings.md#run-elasticsuite-on-opensearch)

Use `make local-gates` for the Pulumi mocks, harness tests and the Floci AWS,
GCP and Azure suites. The tests exercise the implemented API contracts.
Real OIDC federation, managed TLS, HA failover, cloud restore times and cost
need deployment-specific evidence. [Pulumi unit tests](https://www.pulumi.com/docs/iac/concepts/testing/unit/)

## Rehearse a release

Start with a staging deployment, record its image digest and check a shopper journey.
Restore a recent database and media snapshot to an empty destination, then measure
how long it takes to return the shop to service. The archived Google Cloud run lists its exact service settings. [Staging walkthrough](onboarding.md) ·
[Database and media rehearsal](weekend-migrate.md) · [Archived Google Cloud configuration](evidence/gcp-autopilot-alpha-configuration.md)

## Staging, UAT and pull requests

Keep the application image, PHP and dependency selectors at project level. Change replicas, database availability and other capacity settings in each environment. Set a smaller preset on staging and PR previews, and choose standard or high availability for UAT. Compare the full result with `magelift env diff staging production`.

The generated GitHub workflow builds in the shop’s registry. After reviewing the current head of a same-repository PR to `main`, apply `magelift-preview` to deploy that revision. Removing the label schedules teardown. After reviewing a later head, reapply it to authorize that revision. Its stack is keyed by repository and PR number and its domain is derived from the preview domain. The workflow uses the base revision of the PHP project for privileged planning and deployment. Use the `preview-build` GitHub environment for the registry identity and `preview` for deployment. Configure required reviewers for these two environments. Stable builds use the `build` environment. Configure their cloud roles and GitHub environment protections before enabling the workflow.

The starter sets `previewTtl` to `168h`, or seven days. CI counts from the pull
request event timestamp, so retrying the same event keeps the expiration.
Closing the PR requests teardown. The scheduled sweep uses the recorded owner to
clean up expired previews. The monthly budget informs the cost estimate. Check
provider billing and set account alerts to track spend.

An AWS team with an existing Fargate cluster and VPC can place `target.aws.previewPool.clusterArn` under its preview environment and supply `target.aws.existing.network` and the public, private and data subnet IDs. The preview reads the shared cluster and owns its own ECS services. Bootstrap the authored preview environment once. PRs have distinct Pulumi stacks and lock records in its state backend. Keep the compute pool in a separately managed stack. Use separate databases, secrets, storage and service identities for PRs that can change code.

`magelift env status staging` prints the configured Magento GraphQL path and domain and links to the target cloud’s operational consoles. Use the provider’s logs, metrics, dashboards and activity history to inspect the running resources. [CLI command reference](cli-reference.md)


GKE previews can reference an existing regional cluster using
`target.gcp.previewPool`. Supply `cluster`, `network`, `subnet`,
`networkPolicyEnforced: true` and `ingressCidrs` for traffic reaching web pods,
including load balancers and health checks. AKS uses `target.azure.previewPool`
with `clusterName`, `nodeSubnetId`, `mysqlSubnetId` and `mysqlDnsZoneId`.
The shared AKS cluster must already reach the delegated subnet and linked private DNS
zone. See the Azure setup in [Bootstrap](bootstrap.md).

Both Kubernetes adapters create a per-preview namespace and ingress policies.
The cluster must enforce NetworkPolicy. Outbound connectivity to managed
services and DNS is still available. Use separate cloud credentials and data
for each preview. Shared compute reduces cluster cost. An AKS preview still
owns a MySQL server, Key Vault and Azure Files storage. [Kubernetes network policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
