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

Production requests need an approval and a digest-pinned signed image. Deployment
re-verifies the promoted digest against its recorded certificate identity and OIDC
issuer before any infrastructure mutation. A rollback is another forward deployment.
MageLift does not attempt to reverse database migrations, so rollback commands require
an explicit acknowledgement of that limitation.

Preview environments must have a TTL and budget before infrastructure is created.
Protected environments reject destroy requests until protection is removed.

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
must be unprotected explicitly before they can be destroyed. Set
`PULUMI_BACKEND_URL` when the project uses a DIY or local Pulumi backend. The AWS
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

`magelift promote --digest <registry>@sha256:<digest>` verifies the image signature
before it records a release. Pass the expected certificate identity and OIDC issuer
with `--certificate-identity` and `--certificate-oidc-issuer`. Use `--from` to name
the environment that supplied the digest. Successful deployments append a deploy
event to the same journal. MageLift writes the release history to
`.magelift/releases/<environment>.jsonl` with an atomic file replacement; these
commands do not contact AWS.

`magelift history` reads that journal. `magelift rollback --to-sequence <number>`
selects an earlier signed digest, verifies it again, runs the same candidate-task and
service workflow with that digest, and records a new release that points to it. It
does not delete history or reverse database migrations. Promote and rollback both
require `--yes` when the selected environment has `class: production`.

`magelift cost` is account-free by default and is routed through the selected
provider's `CostEstimator` adapter (same boundary as logs/ops — ADR 0002). AWS ECS
Fargate reports catalog capacity and budget inputs; `magelift cost --live` queries
the AWS Price List API for on-demand capacity in the selected region and separates
priced resources from capacity that remains an estimate and products AWS could not
match. Experimental targets return “not supported yet” until they ship an adapter.
Data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT processing
remain unsupported because they depend on real workload measurements. A budget is
never presented as a forecast.

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
`--mode runtime` reads ECS service and task state through the AWS API and checks the
desired/running count, primary deployment rollout, and task health. MageLift does
not infer runtime health from configuration or stack outputs. Exit code 4 means a
check failed, while exit code 3 means the requested evidence is unavailable.
Production stacks additionally run a CloudWatch Synthetics check against the public
`/health` endpoint every five minutes. The canary's `SuccessPercent` alarm treats
missing data as a failure and sends notifications through the configured SNS topic.
Run artifacts are encrypted, versioned, and lifecycle-managed in the private
synthetic-artifact bucket. The CLI health command reports ECS evidence; canary run
history and screenshots remain in CloudWatch Synthetics.

`magelift ci generate --magelift-version vX.Y.Z` writes a deterministic GitHub Actions
workflow for the configured environment matrix. The generated workflow validates every
environment, builds and signs one digest, previews opt-in pull requests labeled
`magelift-preview`, destroys a labeled preview when its pull request closes, deploys
`staging` on main, sweeps expired previews hourly, and gates a manual `production`
run with the GitHub production environment. Set `MAGELIFT_AWS_REGION`, set
`MAGELIFT_BUILD_ROLE_ARN` to `identity.buildRoleArn`, and set each environment role
variable to its `identity.ciRoleArn` returned by `magelift bootstrap`,
`MAGELIFT_PULUMI_BACKEND_URL`, image references, and
the release certificate identity before enabling those jobs. `magelift ci validate`
detects a missing, edited, or configuration-drifted workflow without contacting AWS.

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
`magelift tunnel` is not exposed for the Fargate-only v1 runtime because there is no
managed bastion or inbound network path to forward through.

Local development stays account-free. Run `magelift dev init` once to create
`.magelift/compose.local.yml`, then use `magelift dev up --service database` for
the lightweight database and cache loop, or `magelift dev up --service app` for
the full Magento stack with FrankenPHP, OpenSearch, and RabbitMQ. `magelift dev
status`, `magelift dev logs`, and `magelift dev exec --service app -- <command>`
complete the workflow. `magelift dev reset --yes` removes all local capability
volumes. Image and service versions can be pinned in the generated Compose file
or through their `MAGELIFT_LOCAL_*_IMAGE` variables. The generated credentials and
loopback-only ports are for local development only; do not reuse them in a
deployed environment. The FrankenPHP classic app also listens on
`https://localhost:8443` with Caddy's internal development CA. Use
`MAGELIFT_LOCAL_HTTPS_PORT` to change the host port. The certificate is
intentionally local and ephemeral; trust the CA only in a development profile,
and do not use it as production TLS evidence.

For a Magento repository with dependencies already installed, run
`MAGELIFT_LOCAL_ADMIN_PASSWORD='...' magelift dev seed`. The command starts the
app profile, runs `bin/magento setup:install` with local capability defaults, and
writes the administrator password only to `.magelift/local.env` with mode 0600.
The generated `.magelift/` directory is ignored by Git. If `app/etc/env.php`
already exists, seeding reports that the project is installed and does not run a
second installation. The password must be at least 16 alphanumeric characters;
the command never prints it or places it in the Docker command arguments.

Deployed tasks use a non-secret `env.php` scaffold from the runtime image. ECS
injects the configured Magento encryption key and managed database JSON fields
through Secrets Manager references, then exposes Adobe's `MAGENTO_DC_*` environment
configuration before the application starts. Configure
`target.aws.encryptionKeySecretArn` with a stable secret and never commit its value.

OpenSearch wiring (task role + pinned AWS SigV4 proxy sidecar + Magento loopback
listener) is covered by Pulumi/unit mocks. The public-tag gate uses that offline
proof plus free-tier `searchMode: disabled` acceptance and prior Chantelle Terraform
OpenSearch ops ([sources/chantelle-opensearch.md](sources/chantelle-opensearch.md)).
That path used ElasticSuite without SigV4 — do not equate it with MageLift’s proxy.
Live MageLift Magento search (index/query/reconnect/IAM) remains a post-tag paid
acceptance checklist; Floci does not prove that data-plane.

Use `make floci-test` for account-free bootstrap, state, lock, versioned-media
restore, ECS runtime health, ephemeral candidate registration, Secrets Manager, and
CloudWatch Logs tests. It covers the AWS SDK paths that Floci implements. It does not
replace a real-AWS pass: GitHub OIDC provider operations and final managed-service
behavior still need a sparse local acceptance run when credits allow. See
[Local AWS acceptance](aws-acceptance.md). The composed Pulumi target is tested
separately with provider mocks, including preview, standard, and high-availability
resource graphs.
