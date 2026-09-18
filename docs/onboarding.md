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
- **CI service account API grants (CI only).** `magelift bootstrap`
  mints the workload identity pool, provider, and service account
  plus the repository impersonation binding. Granting the service
  account API roles on your project stays an operator IAM step;
  cover at least the services admission checks (artifactregistry,
  compute, container, sqladmin, secretmanager, storage,
  serviceusage, plus billing read and logging/monitoring for
  observability).
- **Your Magento 2.4.9 tree in git.** Builds inspect the git
  checkout holding `magelift.yaml` and refuse detached or
  uncommitted trees; the shop needs a remote for provenance
  and a committed `composer.lock`.
- **Docker with buildx on the build host.** `docker buildx version`
  must answer; base images build for `linux/amd64`.
- **A registry you can push to.** The step below creates an
  Artifact Registry repository; adapt the commands if you use
  another registry.
- **Domain ownership and DNS.** The zone and its records stay
  operator-managed: nothing in the stack creates DNS. After deploy,
  point your names at the load balancer address from
  `magelift outputs` (see Verify).
- **Composer auth.** Builds need Composer credentials as a
  `gcp-secret-manager://` reference. Put the reference in YAML,
  never the value.
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

No stable release exists yet, so install an explicit prerelease.
Pick the tag from [Releases](https://github.com/magelift/magelift/releases):

```sh
curl -fsSL https://magelift.dev/install.sh | MAGELIFT_VERSION=v0.1.0-alpha.1-rc.13 sh
magelift version
```

The installer verifies the Sigstore bundle plus archive checksum
before installing; see [Install](install.md) for the full story.

### 2. Template

Run the `magelift` steps from your shop checkout (here `~/shop`);
the base-image build later runs from a separate MageLift tree
(`~/magelift-src`).

```sh
cd ~/shop
magelift init --provider gcp
```

Fill in your GCP project and domains in `magelift.yaml`. The
starter matches the alpha recipe (preview preset, baked static
content, recipe search); staging and production inherit with
larger presets. Commit the result: builds refuse uncommitted
trees.

### 3. Validate

```sh
magelift doctor
magelift config validate --env preview
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

Admission checks that the project APIs are enabled; enable them
first (one command, idempotent):

```sh
gcloud services enable artifactregistry.googleapis.com \
  compute.googleapis.com container.googleapis.com \
  cloudbilling.googleapis.com memorystore.googleapis.com \
  secretmanager.googleapis.com serviceusage.googleapis.com \
  sqladmin.googleapis.com storage.googleapis.com \
  logging.googleapis.com monitoring.googleapis.com \
  --project=PROJECT_ID
```

```sh
magelift bootstrap --env preview
```

Prepares the GCP project: state bucket with versioning, WIF pool,
provider, and service account. GCP needs no access-log bucket
flag (that one is AWS-only).

### 6. Build the application image

Releases never publish base images for prerelease tags, so fetch
the pinned MageLift tree from the release tarball and build
yours once (the installer ships a binary; sources come
separately):

```sh
TAG=v0.1.0-alpha.1-rc.13
mkdir -p ~/magelift-src
curl -fsSL "https://github.com/magelift/magelift/archive/refs/tags/$TAG.tar.gz" \
  | tar -xz -C ~/magelift-src --strip-components=1
cd ~/magelift-src
```

Create the registry repositories and authenticate Docker, then
build and push the bases:

```sh
gcloud artifacts repositories create shop-bases \
  --repository-format=docker --location=europe-west1 \
  --project=PROJECT_ID
gcloud artifacts repositories create shop \
  --repository-format=docker --location=europe-west1 \
  --project=PROJECT_ID
gcloud auth configure-docker europe-west1-docker.pkg.dev
REG=europe-west1-docker.pkg.dev/PROJECT_ID/shop-bases
docker buildx build --platform linux/amd64 -f images/php-nginx/Dockerfile \
  --target builder -t $REG/magelift-builder:8.5 --push .
docker buildx build --platform linux/amd64 -f images/php-nginx/Dockerfile \
  --target runtime -t $REG/magelift-nginx:8.5 --push .
```

Record the pushed digests:

```sh
gcloud artifacts docker images list \
  $REG/magelift-builder --include-tags --project=PROJECT_ID
gcloud artifacts docker images list \
  $REG/magelift-nginx --include-tags --project=PROJECT_ID
```

Back in the shop tree, build and push the immutable shop image:

```sh
cd ~/shop
magelift build --push \
  --image europe-west1-docker.pkg.dev/PROJECT_ID/shop/shop:preview \
  --builder-image "$REG/magelift-builder@sha256:BUILDER_DIGEST" \
  --runtime-image "$REG/magelift-nginx@sha256:RUNTIME_DIGEST" \
  --platform linux/amd64
```

The command prints the pushed digest. Pin it as
`target.gcp.imageDigest` (or pass `--digest` to deploy and
promote). Tags move; only digests deploy.

### 7. Deploy

```sh
magelift deploy --env preview --yes
```

Preview deploys email-disabled unless you set the `email` block.
Every failure carries a typed cause with a next step; see
When it fails.

### 8. Verify, per surface

```sh
magelift --env preview health --mode runtime
magelift --env preview outputs --output json
```

- **HTTPS:** `curl -sI https://preview.example.com/health`
  returns 200.
- **Assets:** prove baked static content serves by fetching one
  stylesheet the homepage references:
  ```sh
  asset=$(curl -s https://preview.example.com/ | grep -o 'static/[^"]*\.css' | head -1)
  curl -sI "https://preview.example.com/$asset" | head -1
  ```
  Expect `200`.
- **Catalog:** create one simple product in admin (Content is
  empty on a fresh shop; search and media checks need it).
- **Search:** reindex, then query the storefront:
  `magelift --env preview exec --service web -- bin/magento indexer:reindex catalogsearch_fulltext`,
  then `curl -s 'https://preview.example.com/catalogsearch/result/?q=<product>'`
  returns the product.
- **Cron/consumers:** the scheduler runs in the cluster whether
  your terminal is open or not; `magelift --env preview logs --service cron`
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
- **Cost:** `magelift --env preview cost` prints account-free
  capacity with unpriced items listed; `--live` is not wired on
  GCP and errors rather than guessing. Budgets alert, never cap.

### 9. Expiry

Previews carry `expiresAt`. In production a scheduled job owns
expiry: the generated CI workflow sweeps on its cron using the
same credentials as deploy. To exercise the mechanism now:

```sh
magelift env sweep --dry-run
magelift env sweep --yes
```

The sweep destroys only expired, unprotected previews and
reports residual cost (retained backups with their horizon;
amounts come from provider billing, never estimates).

### 10. Teardown

```sh
magelift destroy --env preview --yes
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
