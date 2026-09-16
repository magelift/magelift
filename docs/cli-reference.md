# CLI reference

This page is generated from the Cobra command tree. Do not edit it by hand.

## Global options

```text
      --config string               configuration file (default "magelift.yaml")
      --env string                  environment name
      --no-interaction              never prompt for input
  -o, --output string               output format: table, json, or yaml (default "table")
      --preview-branch string       source branch metadata for a preview identity
      --preview-commit string       commit digest metadata for a preview identity
      --preview-domain string       domain metadata for a preview identity
      --preview-generation uint     deployment generation for a preview identity
      --preview-number int          pull-request number for a preview identity
      --preview-repository string   canonical repository slug for a pull-request preview
  -v, --verbose count               increase diagnostic verbosity
  -y, --yes                         confirm destructive actions
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
      --access-log-bucket string   existing AWS log bucket (AWS only)
      --github-owner string        optional GitHub owner for Actions deploys
      --github-repo string         optional GitHub repository for Actions deploys
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
## magelift certification

Inspect provider cells and verify acceptance evidence

```text
magelift certification
```
### magelift certification cells

Expand one provider target into architecture cells

```text
magelift certification cells [flags]
```

Options:

```text
      --edition string   Magento edition: open-source or commerce (default "open-source")
      --preset string    environment preset (default "preview")
      --release string   exact Adobe Commerce release (default "2.4.9")
      --target string    target ID, such as aws/ecs-fargate
```
### magelift certification docs

Generate source-dated release documents from the catalog and sealed evidence

```text
magelift certification docs [flags]
```

Options:

```text
      --evidence-file string   sealed JSONL evidence file to include
      --output-dir string      directory for generated Markdown documents
      --run-id string          run ID when the evidence file contains more than one run
```
### magelift certification plan

Print a side-effect-free certification execution plan

```text
magelift certification plan [flags]
```

Options:

```text
      --edition string   Magento edition: open-source or commerce (default "open-source")
      --preset string    environment preset (default "preview")
      --release string   exact Adobe Commerce release (default "2.4.9")
      --target string    target ID, such as gcp/gke-standard
```
### magelift certification seal

Seal unsealed acceptance candidates with the core evidence contract

```text
magelift certification seal [flags]
```

Options:

```text
      --file string          unsealed JSONL evidence candidates
      --output-file string   sealed JSONL output file (default: input with .sealed.jsonl suffix)
```
### magelift certification targets

List provider architecture descriptors

```text
magelift certification targets
```
### magelift certification verify

Verify sealed JSONL acceptance evidence

```text
magelift certification verify [flags]
```

Options:

```text
      --file string                 JSONL evidence file
      --required-cell stringArray   required stable cell ID; repeat for multiple cells
      --run-id string               run ID when the file contains more than one run
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
      --budget   read the provider budget definition and alert thresholds when supported
      --live     query current provider on-demand prices when the adapter supports it
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
      --ack-maintenance-drain   attest backup, maintenance mode, and drained writers for production schema risk (see docs/operations.md)
      --digest string           override the configured immutable image digest
      --infra-only              update the infrastructure graph only (skip Magento migrate/health)
```
## magelift destroy

Destroy the selected environment

```text
magelift destroy [flags]
```

Options:

```text
      --destroy-backups   also destroy leftover provider backups after the environment is gone; GCP Cloud SQL leftovers are deleted even when the backup policy is disposable, AWS snapshots are still refused
      --skip-lock         skip the provider distributed state lock (acceptance cleanup only)
```
## magelift doctor

Check this project and print the next Magelift command

```text
magelift doctor [flags]
```

Options:

```text
      --install-dependencies   install missing allowlisted dependencies after confirmation
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
      --dump string                path to a MySQL dump to seed after first deploy (ADR 0010)
      --expires-at string          preview expiration as RFC3339
      --monthly-budget-cents int   maximum monthly AWS budget in cents
      --preset string              infrastructure preset
      --protection                 protect destructive operations
```
### magelift env destroy

Destroy an environment and remove its configuration overlay

```text
magelift env destroy <environment> [flags]
```

Options:

```text
      --destroy-backups   also destroy leftover provider backups after the environment is gone; GCP Cloud SQL leftovers are deleted even when the backup policy is disposable, AWS snapshots are still refused
```
### magelift env dump

Create a Magento database dump from a live environment and write it locally

```text
magelift env dump <environment> [flags]
```

Options:

```text
      --sanitize    hash mailbox addresses in the dump (not certified anonymous)
      --to string   local .sql or .sql.gz path to write (required)
```
### magelift env import-dump

Import the environment seedDump into the target database

```text
magelift env import-dump <environment>
```
### magelift env list

List configured environments

```text
magelift env list
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
      --off   allow destructive operations
      --on    protect destructive operations
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
      --before string   treat environments expiring at or before this RFC3339 time as expired
      --dry-run         report expired environments without changing infrastructure or configuration
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
      --container string   container name (default "web")
      --service string     logical service: web or cron (default "web")
      --session-only       print the resolved session command without starting it
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

Create a starter magelift.yaml for Magento's PHP storefront

```text
magelift init [flags]
```

Options:

```text
      --config-out string   write generated YAML to PATH for review (default: --config path)
      --from-acc            generate magelift.yaml from Adobe Commerce Cloud config
      --from-upsun          generate magelift.yaml from Upsun / Platform.sh config
      --provider string     starter provider: aws or gcp (certified starters only) (default "aws")
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
      --agent string     agent path: codex, claude, cursor, or generic (default "generic")
      --backend string   installer backend: direct or skills-cli (default "direct")
      --mode string      installation mode: auto, copy, or symlink (default "auto")
      --replace          replace an unrelated skill already at the destination
      --scope string     installation scope: project or global (default "project")
  -s, --skill strings    install only the named skill; repeat the flag for more than one
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
      --local-port int    local loopback port (defaults to the target port, or 8080 for app)
      --remote-port int   provider-side port (defaults to the target's verified service port)
      --session-only      print the resolved tunnel command without starting it
      --target string     private target: app, db, db-ui, queue, queue-ui, search, or search-ui (default "app")
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
