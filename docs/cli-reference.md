# CLI reference

This page is generated from the Cobra command tree. Do not edit it by hand.

## Global options

```text
      --config string                 configuration file (default "magelift.php")
      --env string                    environment name
      --no-interaction[=true|false]   never prompt for input
  -o, --output string                 output format: table, json, or yaml (default "table")
      --preview-branch string         source branch metadata for a preview identity
      --preview-commit string         commit digest metadata for a preview identity
      --preview-domain string         domain metadata for a preview identity
      --preview-expires-at string     absolute RFC3339 expiration for this pull request preview
      --preview-generation uint       deployment generation for a preview identity
      --preview-number int            pull-request number for a preview identity
      --preview-repository string     canonical repository slug for a pull-request preview
      --preview-updated-at string     pull request event time used with previewTtl
  -v, --verbose count                 increase diagnostic verbosity
  -y, --yes[=true|false]              confirm destructive actions
```
## magelift audit

Export a control-posture matrix with evidence pointers and no secret values

```text
magelift audit
```
## magelift benchmark

Measure a Magento workload for preset sizing

```text
magelift benchmark
```
### magelift benchmark run

```text
magelift benchmark run [flags]
```

Options:

```text
      --cache string              cache implementation and version under test
      --catalog string            benchmark catalog name
      --catalog-revision string   benchmark catalog revision or commit
      --concurrency int           number of concurrent workers (default 10)
      --database string           database engine and version under test
      --duration duration         measurement duration (default 30s)
      --edition string            Magento edition: open-source or commerce (default "open-source")
      --magento-version string    Magento version under test
      --mix string                request mix as comma-separated path=weight entries (default "/=100")
      --php-version string        PHP version under test
      --queue string              queue implementation and version under test
      --search string             search engine and version under test
      --url string                Magento base URL to measure
```
## magelift bootstrap

Prepare this cloud account for Magento

```text
magelift bootstrap [flags]
```

Options:

```text
      --access-log-bucket string            existing AWS log bucket (AWS only)
      --apply-ci-permissions[=true|false]   apply reviewed GCP CI IAM grants (GCP only)
      --configure-github[=true|false]       write allowlisted GitHub repository variables after CI permissions are applied (GCP only)
      --github-owner string                 optional GitHub owner for Actions deploys
      --github-repo string                  optional GitHub repository for Actions deploys
```
## magelift build

Build an immutable Magento application image

```text
magelift build [flags]
```

Options:

```text
      --builder-image string   advanced builder image override (pair with --runtime-image; registry digest required with --push)
      --export-recipe string   export the bundled build recipe without building or publishing
      --image string           target image reference for a pushed build
      --platform strings       target platform, repeatable or comma-separated
      --push[=true|false]      push the image and publish build attestations
      --runtime-image string   advanced runtime image override (pair with --builder-image; registry digest required with --push)
      --source string          shop checkout to build when the PHP project is stored separately
      --source-url string      HTTPS Git source URL used for provenance
```
## magelift cache-flush

Run the Magento cache-flush operation on the web workload

```text
magelift cache-flush
```
## magelift ci

Generate and validate GitHub Actions workflows

```text
magelift ci
```
### magelift ci generate

Generate the GitHub Actions workflow

```text
magelift ci generate [flags]
```

Options:

```text
      --magelift-version string   MageLift release version installed by CI (default "dev")
      --path string               workflow output path
      --replace[=true|false]      replace an existing workflow that differs from the MageLift template
```
### magelift ci validate

Validate the GitHub Actions workflow

```text
magelift ci validate [flags]
```

Options:

```text
      --check-template[=true|false]   require the workflow to match the current MageLift template exactly
      --magelift-version string       MageLift release version installed by CI (default "dev")
      --path string                   workflow output path
```
## magelift cleanup

Reconcile interrupted environment and acceptance cleanup

```text
magelift cleanup
```
### magelift cleanup claim

Record an owned resource before creating it

```text
magelift cleanup claim [flags]
```

Options:

```text
      --identity string   provider identity if already known
      --kind string       resource kind, such as rdb-instance
      --ledger string     path to the cleanup ledger JSON file
      --marker string     ownership marker
      --name string       provider name used before the identity exists
      --profile string    local CLI profile name
      --project string    provider project or account
      --provider string   provider ID, such as scaleway
      --rank int          deletion rank; snapshots are lower than source instances
      --region string     provider region
      --role string       resource role: source, restore, or snapshot
      --run-id string     stable run identity
```
### magelift cleanup plan

Show owned resources a reconcile would delete

```text
magelift cleanup plan [flags]
```

Options:

```text
      --dir string      directory of cleanup ledger JSON files
      --ledger string   path to one cleanup ledger JSON file
```
### magelift cleanup reconcile

Delete claimed resources left behind by an interrupted run

```text
magelift cleanup reconcile [flags]
```

Options:

```text
      --dir string      directory of cleanup ledger JSON files
      --ledger string   path to one cleanup ledger JSON file
```
### magelift cleanup record

Bind a provider identity after create succeeds

```text
magelift cleanup record [flags]
```

Options:

```text
      --identity string   provider identity returned by create
      --kind string       resource kind, such as rdb-instance
      --ledger string     path to the cleanup ledger JSON file
      --name string       claimed provider name
```
## magelift compatibility

Inspect the Adobe and MageLift compatibility catalog

```text
magelift compatibility
```
### magelift compatibility catalog

List release and service requirements

```text
magelift compatibility catalog [release]
```
### magelift compatibility validate

Validate one catalog service choice

```text
magelift compatibility validate [flags]
```

Options:

```text
      --component string   catalog component, such as database or search
      --option string      service option, such as mysql or opensearch
      --release string     exact Adobe Commerce release, such as 2.4.8-p5
      --version string     service version when the catalog lists versions
```
## magelift completion

Generate shell completion

```text
magelift completion [bash|zsh|fish|powershell]
```
## magelift config

Inspect configuration

```text
magelift config
```
### magelift config effective

Print effective configuration with provenance

```text
magelift config effective
```
### magelift config explain

Explain where an effective value came from

```text
magelift config explain [path]
```
### magelift config validate

Validate configuration

```text
magelift config validate
```
## magelift cost

Describe the selected environment's cost inputs

```text
magelift cost [flags]
```

Options:

```text
      --budget[=true|false]   read the provider budget definition and alert thresholds when supported
      --live[=true|false]     query current provider on-demand prices when the adapter supports it
```
## magelift cron-run

Run the Magento cron-run operation on the web workload

```text
magelift cron-run
```
## magelift deploy

Deploy the selected environment

```text
magelift deploy [flags]
```

Options:

```text
      --ack-maintenance-drain[=true|false]   attest backup, maintenance mode, and drained writers for production schema risk (see docs/operations.md)
      --digest string                        override the configured immutable image digest
      --infra-only[=true|false]              update the infrastructure graph only (skip Magento migrate/health)
```
## magelift destroy

Destroy the selected environment

```text
magelift destroy [flags]
```

Options:

```text
      --destroy-backups[=true|false]   also destroy leftover provider backups after the environment is gone; GCP Cloud SQL leftovers are deleted even when the backup policy is disposable, AWS snapshots are still refused
      --skip-lock[=true|false]         skip the provider distributed state lock (acceptance cleanup only)
```
## magelift dev

Start Magento locally, install an empty shop and check the storefront

```text
magelift dev [flags]
```

Options:

```text
      --search-engine string   Magento search adapter for initial installation, such as opensearch or elasticsuite (default "opensearch")
```
## magelift doctor

Check this project and print the next MageLift command

```text
magelift doctor [flags]
```

Options:

```text
      --export string                       write an allowlisted local diagnostic JSON report to a new file (mode 0600)
      --install-dependencies[=true|false]   install missing allowlisted dependencies after confirmation
      --local[=true|false]                  skip cloud credential probes; still run local toolchain checks
```
## magelift edge

Manage explicitly configured external edge services

```text
magelift edge
```
### magelift edge apply

Apply the configured Fastly service and domains

```text
magelift edge apply
```
### magelift edge destroy

Delete only Fastly resources owned by this project environment

```text
magelift edge destroy
```
### magelift edge plan

Print the Fastly edge plan without changing provider state

```text
magelift edge plan
```
### magelift edge purge

Invalidate or purge the selected environment's Magento-facing edge cache

```text
magelift edge purge
```
## magelift env

MageLift env commands

```text
magelift env
```
### magelift env create

Add an environment to the project

```text
magelift env create <environment> [flags]
```

Options:

```text
      --account string             AWS account ID
      --branches strings           Git branches mapped to this environment
      --class string               environment class
      --domain string              environment domain
      --dump string                path to a MySQL dump for explicit import before application deploy
      --expires-at string          preview expiration as RFC3339
      --monthly-budget-cents int   maximum monthly AWS budget in cents
      --preset string              infrastructure preset
      --protection[=true|false]    protect destructive operations
```
### magelift env destroy

Destroy an environment and remove its configuration overlay

```text
magelift env destroy <environment> [flags]
```

Options:

```text
      --destroy-backups[=true|false]   also destroy leftover provider backups after the environment is gone; GCP Cloud SQL leftovers are deleted even when the backup policy is disposable, AWS snapshots are still refused
```
### magelift env diff

Compare two resolved environments and show where each difference came from

```text
magelift env diff FROM TO
```
### magelift env dump

Create a Magento database dump from a live environment and write it locally

```text
magelift env dump <environment> [flags]
```

Options:

```text
      --sanitize[=true|false]   hash email addresses in the dump (other personal data may remain)
      --to string               local .sql or .sql.gz path to write (required)
```
### magelift env import-dump

Import a dump into the environment database

```text
magelift env import-dump <environment> [flags]
```

Options:

```text
      --from string   local SQL or SQL.gz file to import
```
### magelift env list

List configured environments

```text
magelift env list
```
### magelift env media-export

Download the environment media bucket tree to a local directory

```text
magelift env media-export <environment> [flags]
```

Options:

```text
      --dest string   local directory to download into (required)
```
### magelift env media-sync

Upload a local media tree into the environment media bucket (merge)

```text
magelift env media-sync <environment> [flags]
```

Options:

```text
      --source string   local media directory to upload (required)
```
### magelift env protect

Enable or disable destructive-operation protection

```text
magelift env protect <environment> [flags]
```

Options:

```text
      --off[=true|false]   allow destructive operations
      --on[=true|false]    protect destructive operations
```
### magelift env status

Show environment overlay status including seed dump journal

```text
magelift env status <environment>
```
### magelift env sweep

Destroy expired preview environments

```text
magelift env sweep [flags]
```

Options:

```text
      --before string          treat environments expiring at or before this RFC3339 time as expired
      --dry-run[=true|false]   report expired environments without changing infrastructure or configuration
```
### magelift env ui

Print a time-limited management UI tunnel without starting it

```text
magelift env ui <environment> [flags]
```

Options:

```text
      --target string   management UI: db-ui, queue-ui, or search-ui (default "db-ui")
```
## magelift evidence

Export reconstructable production change evidence without secret values

```text
magelift evidence
```
## magelift exec

Run a command on a Magento workload

```text
magelift exec --service web --container web -- <command> [flags]
```

Options:

```text
      --container string            container name (default "web")
      --service string              logical service: web or cron (default "web")
      --session-only[=true|false]   print the resolved session command without starting it
```
## magelift extensions

Inspect explicitly registered provider extensions

```text
magelift extensions
```
### magelift extensions list

List extension provenance and targets

```text
magelift extensions list
```
## magelift health

Check configuration or deployed stack health evidence

```text
magelift health [flags]
```

Options:

```text
      --mode string   health evidence source: config, outputs, or runtime (default "config")
```
## magelift history

List the local release journal

```text
magelift history
```
## magelift init

Configure a Magento shop with PHP

```text
magelift init [flags]
```

Options:

```text
      --app string                  Magento application to import from a multi-app Upsun project
      --cloud-account string        AWS account ID, Google Cloud project ID, or Azure subscription ID
      --config-out string           output PHP project file for review
      --domain string               production host, also used as the parent of staging, UAT and preview hosts
      --edition string              edition to create with --new (open-source or commerce) (default "open-source")
      --from-acc[=true|false]       import Adobe Commerce Cloud configuration
      --from-compose[=true|false]   read database, cache, search and queue versions from the shop’s Compose file
      --from-upsun[=true|false]     import Upsun or Platform.sh configuration
      --magento-version string      exact Magento 2 release to create with --new (defaults to Composer's selection)
      --new[=true|false]            create a Magento project in an empty directory with Composer
      --provider string             destination cloud (aws, gcp or azure) (default "gcp")
      --region string               default cloud region for the shop
      --storefront-origin string    connect an externally deployed headless storefront at this origin
```
## magelift local

Run the local Magento environment

```text
magelift local
```
### magelift local down

Run local Compose down

```text
magelift local down
```
### magelift local exec

Run a command in a local Compose service

```text
magelift local exec --service app -- <command> [flags]
```

Options:

```text
      --service string   Compose service (default "app")
```
### magelift local init

Create the local Docker Compose environment

```text
magelift local init
```
### magelift local logs

Run local Compose logs

```text
magelift local logs [flags]
```

Options:

```text
      --service string   limit the operation to one Compose service
```
### magelift local reset

Run local Compose reset

```text
magelift local reset
```
### magelift local seed

Install Magento into the local Compose services

```text
magelift local seed [flags]
```

Options:

```text
      --admin-email string         Magento administrator email (default "admin@example.test")
      --admin-firstname string     Magento administrator first name (default "MageLift")
      --admin-lastname string      Magento administrator last name (default "Local")
      --admin-user string          Magento administrator username (default "admin")
      --backend-frontname string   Magento administrator URL segment (default "admin")
      --base-url string            local Magento base URL (default "http://localhost:8080/")
      --search-engine string       Magento search adapter for initial installation, such as opensearch or elasticsuite (default "opensearch")
```
### magelift local status

Run local Compose status

```text
magelift local status
```
### magelift local up

Run local Compose up

```text
magelift local up [flags]
```

Options:

```text
      --service string   limit the operation to one Compose service
```
## magelift login

Verify cloud credentials for the selected environment

```text
magelift login
```
## magelift logs

Read recent Magento application logs

```text
magelift logs [flags]
```

Options:

```text
      --filter string    provider log filter pattern
      --limit int        maximum number of events (default 100)
      --service string   log service: web, deploy, or cron (default "web")
      --since string     duration or RFC3339 start time (default "15m")
      --until string     optional duration or RFC3339 end time
```
## magelift migrate

Move shop data to a Kubernetes environment

```text
magelift migrate
```
### magelift migrate restore

Plan or restore a SQL dump and media into Kubernetes

```text
magelift migrate restore [flags]
```

Options:

```text
      --allow-nonempty[=true|false]   allow importing into a non-empty database
      --ca-secret string              namespace-local Secret containing ca.crt for database TLS
      --context string                destination kubeconfig context
      --database string               destination database name (default "magento")
      --database-host string          database hostname reachable from the namespace
      --database-secret string        namespace-local Secret with password and optionally username keys
      --database-user string          database username when the Secret contains only a password
      --execute[=true|false]          execute the reviewed plan
      --media string                  local media directory to merge into a PVC
      --media-claim string            existing destination media PVC
      --namespace string              destination namespace
      --namespace-uid string          expected namespace UID
      --sql string                    local SQL or SQL.gz dump
      --timeout duration              maximum lifetime of each restore helper, for example 4h (default 1h0m0s)
      --tls string                    database TLS: verify or disabled (default "verify")
```
## magelift outputs

Read outputs from the selected environment

```text
magelift outputs
```
## magelift preview

Preview Magento environment changes without applying

```text
magelift preview
```
## magelift promote

Record a signed image digest as a release

```text
magelift promote [flags]
```

Options:

```text
      --certificate-identity string      advanced: expected signing identity; omit to use the current login
      --certificate-oidc-issuer string   advanced: expected signing issuer; omit to use the current login
      --digest string                    signed registry digest reference
      --from string                      source environment
```
## magelift providers

Download and verify provider plugins

```text
magelift providers
```
### magelift providers install

Download and verify the providers named by magelift.php

```text
magelift providers install [flags]
```

Options:

```text
      --cache-dir string   provider cache directory (default: user cache)
      --lockfile string    provider lockfile path (default: ./magelift.providers.lock, then beside the CLI)
      --version string     release tag to bootstrap the lockfile from when none exists (default: the CLI version)
```
## magelift queue-status

Run the Magento queue-status operation on the web workload

```text
magelift queue-status
```
## magelift reindex

Run the Magento reindex operation on the web workload

```text
magelift reindex
```
## magelift rollback

Deploy a previous signed digest as a new release

```text
magelift rollback [flags]
```

Options:

```text
      --ack-forward-only[=true|false]   acknowledge that rollback never reverses database migrations
      --to-sequence int                 previous release sequence
```
## magelift secret

MageLift secret commands

```text
magelift secret
```
### magelift secret list

List application secret names

```text
magelift secret list
```
### magelift secret remove

Schedule an application secret for deletion

```text
magelift secret remove <name>
```
### magelift secret set

Create or update an application secret

```text
magelift secret set <name> [flags]
```

Options:

```text
      --value-stdin[=true|false]   read the secret value from stdin
```
## magelift sign

Sign a pushed image digest using the current cloud login

```text
magelift sign [flags]
```

Options:

```text
      --digest string                registry digest reference
      --identity-token-file string   advanced: identity token file; omit to use the current gcloud or CI login
```
## magelift skills

List, install, and verify MageLift agent skills

```text
magelift skills
```
### magelift skills install

Install bundled skills for an agent

```text
magelift skills install [flags]
```

Options:

```text
      --agent string           agent path: codex, claude, cursor, or generic (default "generic")
      --backend string         installer backend: direct or skills-cli (default "direct")
      --mode string            installation mode: auto, copy, or symlink (default "auto")
      --replace[=true|false]   replace an unrelated skill already at the destination
      --scope string           installation scope: project or global (default "project")
  -s, --skill strings          install only the named skill; repeat the flag for more than one
```
### magelift skills list

List bundled first-party skills

```text
magelift skills list
```
### magelift skills verify

Verify installed first-party skills without executing them

```text
magelift skills verify [flags]
```

Options:

```text
      --agent string    agent path: codex, claude, cursor, or generic (default "generic")
      --scope string    installation scope: project or global (default "project")
  -s, --skill strings   verify only the named skill; repeat the flag for more than one
```
## magelift ssh

Open a shell on a Magento workload

```text
magelift ssh [flags]
```

Options:

```text
      --command string              shell command to run (default "/bin/sh")
      --container string            container name (default "web")
      --service string              logical service: web or cron (default "web")
      --session-only[=true|false]   print the resolved session command without starting it
```
## magelift state

MageLift state commands

```text
magelift state
```
### magelift state backup

Create a versioned stack-state backup

```text
magelift state backup
```
### magelift state restore

Restore stack state from a versioned backup

```text
magelift state restore <backup-id>
```
### magelift state status

Show the deployment lock status

```text
magelift state status
```
### magelift state unlock

Remove a stale deployment lock

```text
magelift state unlock
```
## magelift status

Show the selected environment configuration

```text
magelift status
```
## magelift tunnel

Forward a private Magento service to localhost

```text
magelift tunnel [flags]
```

Options:

```text
      --local-port int              local loopback port (defaults to the target port, or 8080 for app)
      --remote-port int             provider-side port (defaults to the target's verified service port)
      --session-only[=true|false]   print the resolved tunnel command without starting it
      --target string               private target: app, db, db-ui, queue, queue-ui, search, or search-ui (default "app")
```
## magelift upgrade

Check for or install a signed MageLift CLI release

```text
magelift upgrade [flags]
```

Options:

```text
      --check[=true|false]   check for an update without replacing the executable
      --version string       install a specific release tag
```
## magelift version

Print version information

```text
magelift version
```
