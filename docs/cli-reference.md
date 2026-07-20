# CLI reference

This page is generated from the Cobra command tree. Do not edit it by hand.

## Global options

```text
      --config string    configuration file (default "magelift.yaml")
      --env string       environment name
      --no-interaction   never prompt for input
  -o, --output string    output format: table, json, or yaml (default "table")
  -v, --verbose count    increase diagnostic verbosity
  -y, --yes              confirm destructive actions
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

Create or reconcile the Pulumi state backend

```text
magelift bootstrap [flags]
```

Options:

```text
      --access-log-bucket string   existing object-storage bucket for state access logs (AWS)
      --github-owner string        GitHub repository owner for the deployment role
      --github-repo string         GitHub repository name for the deployment role
```
## magelift build

Build an immutable Magento application image

```text
magelift build [flags]
```

Options:

```text
      --builder-image string   builder image pinned by registry digest
      --image string           target image reference for a pushed build
      --platform strings       target platform, repeatable or comma-separated
      --push                   push the image and publish build attestations
      --runtime-image string   runtime image pinned by registry digest
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
```
### magelift ci validate

Validate the GitHub Actions workflow

```text
magelift ci validate [flags]
```

Options:

```text
      --magelift-version string   MageLift release version installed by CI (default "dev")
      --path string               workflow output path
```
## magelift completion

Generate shell completion

```text
magelift completion [bash|zsh|fish|powershell]
```
## magelift config

Inspect and migrate configuration

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
### magelift config migrate

Migrate configuration to the current schema

```text
magelift config migrate
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
      --live   query current AWS on-demand prices
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
      --digest string   override the configured immutable image digest
```
## magelift destroy

Destroy the selected environment

```text
magelift destroy
```
## magelift dev

Run the local Magento development environment

```text
magelift dev
```
### magelift dev down

Run local Compose down

```text
magelift dev down
```
### magelift dev exec

Run a command in a local Compose service

```text
magelift dev exec --service app -- <command> [flags]
```

Options:

```text
      --service string   Compose service (default "app")
```
### magelift dev init

Create the local Docker Compose environment

```text
magelift dev init
```
### magelift dev logs

Run local Compose logs

```text
magelift dev logs [flags]
```

Options:

```text
      --service string   limit the operation to one Compose service
```
### magelift dev reset

Run local Compose reset

```text
magelift dev reset
```
### magelift dev seed

Install Magento into the local Compose services

```text
magelift dev seed [flags]
```

Options:

```text
      --admin-email string         Magento administrator email (default "admin@example.test")
      --admin-firstname string     Magento administrator first name (default "MageLift")
      --admin-lastname string      Magento administrator last name (default "Local")
      --admin-user string          Magento administrator username (default "admin")
      --backend-frontname string   Magento administrator URL segment (default "admin")
      --base-url string            local Magento base URL (default "http://localhost:8080/")
```
### magelift dev status

Run local Compose status

```text
magelift dev status
```
### magelift dev up

Run local Compose up

```text
magelift dev up [flags]
```

Options:

```text
      --service string   limit the operation to one Compose service
```
## magelift doctor

Check local project readiness

```text
magelift doctor
```
## magelift env

MageLift env commands

```text
magelift env
```
### magelift env create

Add an environment overlay to magelift.yaml

```text
magelift env create <environment> [flags]
```

Options:

```text
      --account string             AWS account ID
      --branches strings           Git branches mapped to this environment
      --class string               environment class
      --domain string              environment domain
      --expires-at string          preview expiration as RFC3339
      --monthly-budget-cents int   maximum monthly AWS budget in cents
      --preset string              infrastructure preset
      --protection                 protect destructive operations
```
### magelift env destroy

Destroy an environment and remove its configuration overlay

```text
magelift env destroy <environment>
```
### magelift env list

List configured environments

```text
magelift env list
```
### magelift env protect

Enable or disable destructive-operation protection

```text
magelift env protect <environment> [flags]
```

Options:

```text
      --off   allow destructive operations
      --on    protect destructive operations
```
### magelift env sweep

Destroy expired preview environments

```text
magelift env sweep [flags]
```

Options:

```text
      --before string   treat environments expiring at or before this RFC3339 time as expired
      --dry-run         report expired environments without changing infrastructure or configuration
```
## magelift exec

Run a command on a Magento workload

```text
magelift exec --service web --container web -- <command> [flags]
```

Options:

```text
      --container string   container name (default "web")
      --service string     logical service: web or cron (default "web")
      --session-only       print the resolved session command without starting it
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

Create a starter magelift.yaml

```text
magelift init
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
```
## magelift outputs

Read outputs from the selected environment

```text
magelift outputs
```
## magelift preview

Preview infrastructure changes

```text
magelift preview
```
## magelift promote

Record promotion of a signed immutable digest

```text
magelift promote [flags]
```

Options:

```text
      --certificate-identity string      expected signing certificate identity
      --certificate-oidc-issuer string   expected signing certificate OIDC issuer
      --digest string                    signed registry digest reference
      --from string                      source environment
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
      --ack-forward-only   acknowledge that rollback never reverses database migrations
      --to-sequence int    previous release sequence
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
      --value-stdin   read the secret value from stdin
```
## magelift ssh

Open a shell on a Magento workload

```text
magelift ssh [flags]
```

Options:

```text
      --command string     shell command to run (default "/bin/sh")
      --container string   container name (default "web")
      --service string     logical service: web or cron (default "web")
      --session-only       print the resolved session command without starting it
```
## magelift state

MageLift state commands

```text
magelift state
```
### magelift state backup

Create a versioned Pulumi state backup

```text
magelift state backup
```
### magelift state restore

Restore Pulumi state from a versioned backup

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

Explain why port forwarding is unavailable

```text
magelift tunnel
```
## magelift upgrade

Check for or install a signed MageLift CLI release

```text
magelift upgrade [flags]
```

Options:

```text
      --check            check for an update without replacing the executable
      --version string   install a specific release tag
```
## magelift version

Print version information

```text
magelift version
```
