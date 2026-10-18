---
title: Onboarding a GCP Autopilot shop
description: One documented path from a clean workstation to a working Magento shop on the GCP Autopilot preview recipe.
---

# Onboarding a GCP Autopilot shop

One path from a clean workstation to a working shop on the [alpha
recipe](alpha-recipe.md): GCP GKE Autopilot, Magento 2.4.9, Cloud SQL
MySQL 8.4, Valkey 9, OpenSearch on GKE, database queue, GCS media,
HTTPS load balancer. Follow it in order; every step names its command
and what good looks like. Anything needing a human is in
Prerequisites, not discovered mid-deploy.

## Prerequisites

Do these first. MageLift automates what it honestly can; the rest
needs you (or your registrar, Google billing, Adobe account team).

- **GCP account with billing activated.** Console: Billing, link a
  payment method to the project. There is no automated billing
  setup, and no deploy works without it.
- **Operator credentials.** Deploys consume Application Default
  Credentials, which `gcloud auth login` alone does not provide.
  On a workstation run `gcloud auth application-default login`;
  on CI use workload identity federation instead.
  `magelift doctor` mints a token to prove ADC works and tells
  you which half is missing; on CI runners it skips the check
  explicitly.
- **IAM for whoever runs bootstrap and deploy.** On a workstation
  that is your Application Default Credentials identity: it must
  be able to create bootstrap resources (state bucket, your
  Artifact Registry repository, Secret Manager entries, and
  missing reference APIs when Service Usage allows) and to run
  `magelift deploy`. MageLift does not grant those roles to a
  human login. For GitHub Actions, bootstrap with
  `--github-owner` and `--github-repo` creates separate build and
  deploy federation identities. The first run returns a
  `ciPermissionPlan` (registry-scoped push/pull, state bucket and
  passphrase access, node image pull, and project-wide deployment
  roles). Federation alone does not apply that plan. Review it
  with the project administrator, prefer a dedicated test
  project, then rerun with `--apply-ci-permissions`. Project IAM
  in the plan is not limited to MageLift-named resources; see
  [Bootstrap](bootstrap.md).
- **Git and a committed shop checkout.** Private repositories and detached
  HEADs are accepted. Builds need a recorded commit and Composer lock;
  a local branch or author identity is not required. Pushed builds use an
  immutable HTTPS provenance URL, including for private repositories.
- **Docker with buildx on the build host.** `docker buildx version`
  must answer; base images build for `linux/amd64`. Debian's
  own Docker package is too old for this path. Use Docker's
  current Debian packages, which include the buildx plugin.
- **A registry you can push to.** GCP bootstrap creates your
  Docker Artifact Registry repository in the configured region and
  returns the image destination. CI and builds target that user
  registry (or another registry you configure); MageLift does not
  host application images centrally.
- **Domain ownership and DNS.** The zone and its records stay
  operator-managed: nothing in the stack creates DNS. After deploy,
  point your names at the load balancer address from
  `magelift outputs` (see Verify).
- **Composer auth for private dependencies.** Use the build credential
  reference `env://COMPOSER_AUTH` for CI, or an existing cloud-secret reference.
  Public dependencies can build without credentials. Values stay out of YAML.
- **SMTP relay account (optional).** Alpha sending is operator
  relay: any SMTP relay with host, port, username, and a password
  stored as a `gcp-secret-manager://...` value. Create the relay
  account and the secret yourself, then set the `email` block
  (the starter carries a commented example). Sandbox and
  sending-limit exits on the relay side stay manual. Without a
  relay the shop runs email-disabled, which is a supported
  shape: it sends no mail until you configure sending, so
  plan order and password flows accordingly.

What MageLift does automate, so nobody does it by hand: database
and queue credentials are generated and stored, secrets travel by
reference into Kubernetes Secrets consumed by `SecretKeyRef` only,
and TLS terminates at the managed load balancer.

## Steps

### 1. Install

No release is published yet. Install from source. The Go toolchain
must match [`go.mod`](https://github.com/magelift/magelift/blob/main/go.mod):

```sh
git clone https://github.com/magelift/magelift.git
cd magelift
go install ./cmd/magelift
magelift version
```

Signed archives and the install script apply after the first release.
See [Install](install.md). The infrastructure engine is inside the CLI.

### 2. Template

Run the `magelift` steps from your shop checkout (here `~/shop`).
The installed CLI carries its build recipes; no framework checkout is needed.

```sh
cd ~/shop
magelift init
```

`init` inspects `composer.json`, `composer.lock`, and `bin/magento`
in the shop checkout (the directory that contains your `--config`
file). It infers the locked Magento edition and exact release, then
writes a minimal `magelift.yaml` with one `test` environment on GCP
by default (`--provider aws` keeps the experimental AWS starter).
Fill in `project.name`, `target.gcp.project`, and
`environments.test.domain`. The starter uses the community alpha
recipe (PHP 8.5, nginx-fpm, preview preset, baked static content).
If your locked release needs a different PHP branch, `init` refuses
before writing; author `magelift.yaml` manually instead. Commit the
result: builds refuse uncommitted trees.

### 3. Validate

```sh
magelift doctor
magelift config validate --env test
```

`doctor` checks the config, host tooling, and ADC credentials,
then prints the next action. `config validate` accepts structural
presence; the provider validates target semantics at plan time
and names the field.

### 4. Providers

```sh
magelift providers install
```

Downloads the locked provider plugin plus its bundle into the
cache after checksum and signature verification, and persists
the pinned verifier so later commands need no signing tooling.
The first run with no lockfile bootstraps the CLI's release
version into a reviewable `magelift.providers.lock`.
Review and commit it: builds refuse uncommitted trees,
and the lock is part of the reviewable project state.

```sh
git add magelift.providers.lock magelift.yaml
git commit -m "Pin the provider release"
```

### 5. Bootstrap

```sh
magelift bootstrap --env test
```

Prepares the GCP project: enables missing reference APIs when the
caller can use Service Usage, creates the versioned state bucket and
state passphrase secret, your Artifact Registry repository, and
returns the image destination plus registry host. GCP needs no
access-log bucket flag (that one is AWS-only).

**GitHub CI (optional).** When deploys run in GitHub Actions, pass the
repository slug. Bootstrap resolves immutable owner and repository IDs
through your GitHub login (`GH_TOKEN`, `GITHUB_TOKEN`, or `gh auth
login`) before changing cloud resources:

```sh
magelift bootstrap --env test \
  --github-owner YOUR_OWNER --github-repo YOUR_REPO
```

Bootstrap prepares the owned GKE node account and its registry-reader and
minimum project runtime role, including without GitHub options. It needs
permission to manage project IAM for that binding. It leaves the default
Compute account unchanged.

Read the returned `ciPermissionPlan` before granting CI build and deployment
roles. Creating the federation identities does not attach those deployment roles.

Apply the reviewed grants and write allowlisted nonsecret repository
variables (`MAGELIFT_GCP_*`, `MAGELIFT_REGISTRY`, `MAGELIFT_RELEASE_IMAGE`,
and signing metadata for the generated workflow). `--configure-github`
requires `--apply-ci-permissions` on the same command:

```sh
magelift bootstrap --env test \
  --github-owner YOUR_OWNER --github-repo YOUR_REPO \
  --apply-ci-permissions --configure-github
```

Repository variables are created when absent, left unchanged when they
already hold the same value, and refused when a name exists with a
different value. Bootstrap does not edit workflow files, environment
protection rules, or secrets (Composer auth stays in GitHub secrets when
needed). Configure GitHub environments named `build` and `test` for the
starter workflow; add reviewers on protected environments. Details and
the full variable list are in [Bootstrap](bootstrap.md).

Generate and validate CI from the same `magelift.yaml` you will deploy:

```sh
magelift ci generate
magelift ci validate
```

A development binary must pass `--magelift-version` naming an available
signed release (the same tag you installed in step 1). Generation never
pulls builder images from a central MageLift registry; the workflow builds
from your shop source and pushes to your configured registry.

### 6. Build the application image

Your build host (workstation or the `build` job in CI) constructs the
bundled recipe and publishes the application image. Builder and runtime
stages stay local to the build. After bootstrap, authenticate Docker to
your registry host (for example `gcloud auth configure-docker
REGION-docker.pkg.dev`), then run from the shop:

```sh
magelift build --push \
  --image REGION-docker.pkg.dev/PROJECT_ID/REPOSITORY/IMAGE:candidate
```

Use the image destination from bootstrap output instead of guessing
paths. See [Application builds](builds.md) for optional private cache and
advanced recipe overrides.

The command prints the pushed digest. Pin it as
`target.gcp.imageDigest` (or pass `--digest` to deploy and
promote). Tags move; only digests deploy.

The stack generates a 32-character Magento encryption key on
first deploy and stores it in Secret Manager. Later deploys
leave that value in place. Leave `encryptionKeySecret` unset.
Set it only when a restore must keep a key that already
encrypted shop data.

### 7. Deploy

```sh
magelift deploy --env test --yes
```

The starter keeps email disabled unless you configure the `email` block.
Every failure carries a typed cause with a next step; see
When it fails.

### 8. Verify, per surface

```sh
magelift --env test health --mode runtime
magelift --env test outputs --output json
```

- **HTTPS:** `curl -sI https://test.example.invalid/health`
  returns 200.
- **Assets:** prove baked static content serves by fetching one
  stylesheet the homepage references:
  ```sh
  asset=$(curl -s https://test.example.invalid/ | grep -o 'static/[^"]*\.css' | head -1)
  curl -sI "https://test.example.invalid/$asset" | head -1
  ```
  Expect `200`.
- **Catalog:** create one simple product in admin (Content is
  empty on a fresh shop; search and media checks need it).
- **Search:** reindex, then query the storefront:
  `magelift --env test exec --service web -- bin/magento indexer:reindex catalogsearch_fulltext`,
  then `curl -s 'https://test.example.invalid/catalogsearch/result/?q=<product>'`
  returns the product.
- **Cron/consumers:** the scheduler runs in the cluster whether
  your terminal is open or not; `magelift --env test logs --service cron`
  shows recent runs. The DB queue needs no broker health cell
  on preview.
- **SMTP (relay shops only):** register a new storefront
  customer account in a browser and confirm the confirmation
  email arrives (check the relay's own logs; never print relay
  credentials). Then trigger the storefront forgot-password
  flow for that same customer and confirm the second arrival.
  Delivery evidence is the proof. The alpha acceptance loop
  runs email-disabled, so delivery proof waits for pilots
  with relays.
- **Media:** upload a product image in admin, then fetch its
  storefront URL with plain curl (no cloud credentials):
  image URLs stay app-relative and materialize through
  get.php. Leave `base_media_url` at its default; the
  build rejects shops pointing it at object storage.
  Confirm the object under `media/` in the `mediaBucket`
  from outputs
  (`gcloud storage ls gs://<mediaBucket>/media/`).
  Import/export files share the private bucket under
  `import_export/` and are never URL-addressable.
- **Cost:** `magelift --env test cost` prints account-free
  capacity with unpriced items listed; `--live` is not wired on
  GCP and errors rather than guessing. Budgets alert, never cap.

### 9. Test lifetime

The initial `test` environment has no automatic expiry. Destroy it after the
trial; budgets alert but do not stop spending. Optional preview lifecycle and
scheduled cleanup are a separate pilot path. The single-test CI template does
not create preview or sweep jobs.

### 10. Teardown

```sh
magelift destroy --env test --yes
```

Destroys exactly what the provider created and reports the
teardown; retained backups need the explicit leftover flag (see
`magelift destroy --help`).

## When it fails

Deploy and destroy classify provider faults into stable exits:
bad input fails input-side (2), expired or revoked credentials
name re-authentication (3), version skew names reinstalling the
provider artifacts (3), lock and ownership conflicts name the
holder (3), upstream faults keep redacted provider text (1).
A Pulumi stack collision keeps its dedicated exit (5).
`magelift logs --service web` and `magelift status` show
runtime-side causes; nothing here requires Pulumi or Kubernetes
commands.

## Costs, honestly

Onboarding reports estimates, unpriced lists, alerts, and limits.
`monthlyBudgetCents` is a planning input that feeds budget alerts;
MageLift never caps or stops spend, on any provider. Preview is
the credit-efficient shape; staging and production presets size
up deliberately.
