# What MageLift does

MageLift takes a Magento 2 shop from a Git checkout to a running environment in
your AWS, Google Cloud or Azure account. Start the shop on your laptop, try a
change in a pull-request preview, then deploy an image you tested in staging.
MageLift works with Magento Open Source and licensed Adobe Commerce.

You own the shop repository, cloud account, image registry, credentials and
customer data. Your cloud provider bills your account. Your team decides which
service versions to use, who can deploy, when to switch traffic and how to
recover the shop. [Run and deploy your first shop](getting-started.md) · [Compare hosted Magento options](compare-paas.md)

## Set up a shop

Run `magelift init` in an existing Magento checkout. MageLift reads the
Composer files for the edition, Magento release and relevant packages and
writes `magelift.php`. Use `init --new` to create a shop through Composer in
an empty directory. Adobe Commerce needs a valid entitlement and a Composer
credential reference. The starter sets up preview, staging, UAT and protected
production environments. Presets fill in the capacity and service defaults.
Use `--env` to select an environment or `--config` to use a different project file.

Keep shared settings in the project and override the fields an environment
needs. `magelift config effective --env staging` shows the resolved project.
Use `config explain` to trace a value back to its source and
`magelift env diff staging production` to review differences before deploying.
Environment commands can write an adjacent PHP file while preserving the
handwritten project. `env list` and `env status` help you see what you configured.

Review the project file as code before running it. MageLift evaluates it with
PHP. `compatibility catalog` and `compatibility validate` use the requirements
snapshot shipped with the CLI. A cloud plugin checks whether it can build the requested topology
with the services available in your account and region. [Configuration reference](configuration.md) · [Magento and service versions](compatibility.md)

## Run the shop on your laptop

`magelift dev` writes Docker Compose configuration, starts the selected
services and mounts your shop source into nginx and PHP-FPM. It installs
Magento into an empty local database and checks the storefront and GraphQL.
If MageLift generates an Admin password, find it in `.magelift/local.env`.
Keep that file private. You can use this workflow before connecting a cloud
account.

Compose runs the database, cache, search and queue services you select.
Use a separate session store to test cache and session policies. Optional
Varnish sits in front of nginx. Mailpit captures email on your workstation,
or you can configure a relay. The default ports bind to your laptop. You
can adjust PHP settings, service images and ports in the local configuration.

Use `local init`, `local up` and `local seed` to run the steps individually.
`local status`, `local logs` and `local exec` help you inspect a running shop.
`local down` stops Compose and keeps its volumes. Use `local reset --yes` when
you intend to remove those volumes. [Local development and cloud environments](local-vs-cloud.md)

## Choose a cloud and services

The CLI uses a separate verified executable for each first-party provider.
The provider turns the shop settings into a plan for a runtime, such as
ECS Fargate or GKE Autopilot. Some services come from the cloud provider.
Others can run beside Magento or connect to an endpoint you already own.

| Target | What it can run |
| --- | --- |
| AWS ECS Fargate (`aws`, `ecs-fargate`) | Magento web, cron and queue tasks. RDS MySQL or MariaDB, Aurora, ElastiCache, managed OpenSearch and database or broker queues. Durable media in S3. Can reference an existing VPC or RDS instance. |
| AWS EKS (`aws`, `eks`) | Experimental Kubernetes deployment with EKS Auto Mode and shared workload code. Configure the AWS database and cache, with options for OpenSearch and RabbitMQ in the cluster. Enable it with `MAGELIFT_EXPERIMENTAL_PROVIDERS=1`. |
| Google Cloud GKE Autopilot (`gcp`, `gke-autopilot`) | Kubernetes workloads with Cloud SQL or self-hosted MariaDB, Memorystore or in-cluster stores, OpenSearch and RabbitMQ. Durable media in Cloud Storage. |
| Google Cloud GKE Standard (`gcp`, `gke-standard`) | Experimental version of the GKE topology with node settings for workloads such as multi-node OpenSearch. |
| Azure AKS (`azure`, `aks`) | Experimental Kubernetes deployment using a dedicated or existing cluster. MySQL Flexible Server or self-hosted MariaDB, in-cluster cache, sessions, OpenSearch and RabbitMQ. Durable media on Azure Files. Uses Blob Storage for state and Key Vault or Kubernetes Secrets for credentials. |

AWS ECS also offers Fargate Spot, EC2 Auto Scaling and ECS Managed Instances
as capacity modes. OpenSearch can use a provisioned domain or Serverless.
Queues can use the database, Amazon MQ, or RabbitMQ or Artemis on ECS. The AWS
module can configure NAT, CloudFront, WAF and CloudWatch. GKE has private
networking, native ingress, Cloud CDN, Cloud Armor and Google
Cloud Operations options. AKS can use a load balancer address or ingress and
TLS settings you supply. A dedicated AKS cluster can send telemetry to an existing
Log Analytics workspace. Check the runtime before selecting services or sharing
a cluster between previews.
[Provider support](capability-matrix.md) · [Service choices](service-bindings.md) · [EKS](aws-eks-experimental.md) · [GKE](gcp-experimental.md)

A shop can use Magento's integrated storefront. For a separate Next.js or
Nuxt storefront, use MageLift for the Magento backend and SST for the
frontend. Connect them through the environment's GraphQL API.
[Storefront setup](storefront-recipes.md)

MageLift can also configure cron, consumer workers, search
plugins, separate cache and session policies, and email delivery. SES managed
sending is an AWS option. Other configured mail relays use SMTP. Previews
disable cloud email by default. SQS and Pub/Sub can use a Magento module
already in the shop's Composer lock. [Application settings](configuration.md)

## Build one image for a release

`magelift build` reads a clean shop checkout and its Composer lock. It checks
the requested PHP extensions and Composer version, installs dependencies,
applies the selected patches and runs Magento compilation. It builds static
assets for the configured themes and locales and preserves disabled modules.
Build hooks can run Composer or Magento commands during preparation.

The CLI bundles the nginx/PHP-FPM recipe with pinned upstream image digests.
It selects PHP 8.3, 8.4 or 8.5 for the bundled build. The result is a container
image and a separate manifest recording the source revision, immutable inputs
and image digest. The digest identifies the exact image for staging and
production. Composer credentials are supplied separately during preparation,
and runtime credentials come from the destination's secret store.

A local build loads the result into Docker. In CI, `build --push` sends the
image to your registry and signs the digest with Cosign. BuildKit also
publishes provenance and a software bill of materials. You can inspect the
recipe, give BuildKit a private registry cache or supply your own builder and
runtime images where needed. [Application builds](builds.md) · [Secret references](secrets.md)

## Connect a cloud account and deploy

`magelift login` checks the credentials you already use for the selected
environment. `magelift bootstrap` prepares the provider's prerequisites,
including a place to keep Pulumi state. Pulumi uses that state to record
which infrastructure resources belong to the environment. Depending on the
target, bootstrap can prepare encryption keys, the image registry and CI
identities. Set up the access log bucket for AWS or the Azure state account
as described in the cloud guide.

`magelift providers install` verifies and installs the selected first-party
provider for a signed CLI release. A source build needs a matching trusted
provider artifact and lock. Help and local configuration checks are available
before a provider is installed. The provider executable runs with the CLI
user's permissions. [Install providers](install.md#providers) · [Cloud setup](bootstrap.md)

`magelift preview` shows the proposed infrastructure changes without applying
them. A normal `magelift deploy` checks the plan and image, takes the
environment lock and previews the update. The provider runs a Magento
migration task or job and rolls the web service forward after migration.
MageLift checks the rollout and shop health and records the release. On a
zero-table database it initializes Magento's schema before the normal upgrade.

A successful deployment prints the shop and GraphQL URLs and a command for
reading web logs. Pass `--outputs-file` to write the environment name and
URLs as JSON for a separate storefront or CI job. `magelift outputs` reads
the deployed environment's outputs again.
A production deploy requires approval and a signed image recorded in the
release journal. `magelift promote` verifies and records a digest for a target
environment. `magelift rollback` verifies and redeploys an earlier digest as a
new release. Database recovery is a separate operation. A schema change that
needs downtime also needs an operator-managed maintenance window and a tested
backup. [Deployment and rollback](operations.md)

## Let GitHub build and deploy

`magelift ci generate` writes a shop workflow for the selected provider and
environment set. It can validate changes, build and sign an image in your
registry, deploy staging, and promote a previously built digest to protected
production. Use `ci validate` to check the config and workflow file. When a shop
already has a customized workflow, review the generated candidate before
replacing it.

A reviewed pull request from the shop repository can get its own environment
when someone applies `magelift-preview`. Preview identity comes from the
repository and pull-request number. The starter uses a seven-day expiry.
Removing the label or closing the pull request requests cleanup, and an
hourly sweep handles expired previews. Supported preview pools can reuse
compute while keeping each preview's state, credentials and data separate.
The deployment identity and environment protection come from your GitHub
settings and cloud account. [Configure GitHub Actions](bootstrap.md#generated-ci-and-pull-request-previews) · [Operate previews](operations.md#staging-uat-and-pull-requests)

## Move an existing shop

`magelift init --from-upsun` reads Upsun or Platform.sh configuration.
`--from-acc` reads Adobe Commerce Cloud settings. `--from-compose` can infer
supported dependencies from service image tags. The CLI produces PHP you can
review and lists settings that need manual attention, such as a custom hook
or mount. It preserves the shop's Composer-driven edition and release.

Rehearse the data transfer on staging with the original Magento encryption
key and consistent SQL and media copies. For a new target, `deploy --infra-only`
can prepare infrastructure ahead of the application rollout. `env dump` exports
SQL where a live transport is available. Its sanitize option hashes email
addresses. Review any other personal data before sharing a dump.
`env import-dump` imports into the selected database
and records the result. `env media-export` and `env media-sync` transfer media
on supported object storage paths. `migrate restore` can plan and run SQL and
media transfers through a Kubernetes namespace. It uses the database and
media destination you supply. Review the host, credentials, TLS and
overwrite behavior before executing a restore or switching traffic.
[Import an existing shop](migrating-from-paas.md) · [Rehearse the data migration](weekend-migrate.md)

## Look after a running environment

- Check the configuration and required tools with `doctor`. Use
  `doctor --local` for checks that need no cloud login. Its optional support
  export omits project identifiers, secret values and logs.
- Use `health` to check config, saved outputs or the current workload. Choose
  runtime mode when you need evidence from the running shop. `logs` reads the
  selected web, cron or deployment logs where the provider exposes them.
- Run a command through `exec` or open a shell with `ssh`. ECS uses ECS Exec.
  The Kubernetes path targets a running workload. Shortcuts cover cache flush,
  cron, reindexing and queue status.
- Use `tunnel` to forward a supported private endpoint to localhost.
  `env ui` prints a time-limited management UI tunnel command. Which services
  you can reach depends on the provider and runtime.
- Use `secret list`, `secret set` and `secret remove` on providers that expose
  secret operations. These commands work with references or standard input
  so images and project files can stay free of secret values.
- Run `edge purge` to clear a configured CloudFront, GKE Cloud CDN or Fastly
  cache where the adapter can verify ownership. The Fastly path also has
  plan, apply and destroy commands. Cloud monitoring depends on the runtime.
  The SDK exposes contracts for other edge and telemetry adapters.
- `cost` shows configured capacity and cost inputs where the provider offers
  an estimator. AWS has a live price lookup. Supported budget readers show
  alert thresholds and scope. Check the cloud bill for actual spend.
- `state status` checks the deployment lock. AWS and GCP support versioned
  stack-state backup and restore. That state is separate from the Magento
  database and media. The provider's state and retention settings determine
  what you can restore later.
- `env protect` controls teardown protection. `destroy` checks protection and
  previews deletion before tearing the stack down. `env destroy` also removes
  the environment overlay after teardown. GCP's `cleanup` ledger can reconcile
  claimed resources from an interrupted operation.
- Use `history` to read release records, `evidence` to export production change
  information with secret values omitted, and `audit` to inspect security,
  monitoring and backup settings.

Operation support depends on the runtime. AKS provides
rollout, health, logs and private workload access. Its current plugin
advertises state locking and status. Some additional operations, such as
stack-state backup and cost reporting, are available through other providers.
[Operations guide](operations.md) · [Provider support](capability-matrix.md)

## Find a command

| Task | Commands |
| --- | --- |
| Create or check a project | `init`, `config validate`, `config effective`, `config explain`, `compatibility catalog`, `compatibility validate`, `doctor` |
| Start or inspect a local shop | `dev`, `local init`, `local up`, `local seed`, `local status`, `local logs`, `local exec`, `local down`, `local reset` |
| Install a provider and connect an account | `providers install`, `extensions list`, `login`, `bootstrap` |
| Build, review and release an image | `build`, `sign`, `preview`, `deploy`, `outputs`, `promote`, `rollback`, `ci generate`, `ci validate` |
| Manage environments | `status`, `env list`, `env create`, `env diff`, `env status`, `env protect`, `env sweep`, `env destroy`, `destroy` |
| Transfer data | `env dump`, `env import-dump`, `env media-export`, `env media-sync`, `migrate restore` |
| Diagnose or maintain the deployment | `health`, `logs`, `exec`, `ssh`, `tunnel`, `env ui`, `cache-flush`, `cron-run`, `reindex`, `queue-status` |
| Manage credentials and edge caches | `secret list`, `secret set`, `secret remove`, `edge plan`, `edge apply`, `edge purge`, `edge destroy` |
| Track costs and changes | `cost`, `history`, `evidence`, `audit` |
| Recover infrastructure state and clean up | `state status`, `state backup`, `state restore`, `state unlock`, `cleanup claim`, `cleanup record`, `cleanup plan`, `cleanup reconcile` |
| Set up the CLI and shop agents | `version`, `upgrade`, `completion`, `skills list`, `skills install`, `skills verify` |

Use `magelift --help` for command groups and `magelift COMMAND --help` for flags.
Choose table, JSON or YAML for structured results. The CLI bundles shop skills
for dependencies, local setup, configuration, migration and operations. It
verifies installed skills without executing them. [CLI reference](cli-reference.md) · [Shop skills](https://github.com/magelift/magelift/tree/main/agents)

## Understand the test evidence

Go and PHP tests check config resolution, builds and the Magento lifecycle.
Pulumi mocks inspect the resource graph and Floci receives selected cloud
SDK calls from local tests. The generated workflow is tested with act and a
local registry. Dated records describe real environments, versions and
outcomes. AWS ECS Fargate and GKE Autopilot have recorded Magento runs. Azure
AKS has local resource, API and runtime tests and remains experimental.
Test your own service versions, IAM, public DNS and recovery in staging.
[Provider support](capability-matrix.md) · [Run records](evidence/README.md)

For package ownership and the provider protocol, see the architecture guide.
Use the provider guide to propose a new cloud or connect an existing service
through the SDK. [Architecture](architecture.md) · [Add a provider](adding-a-provider.md)
