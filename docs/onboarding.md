---
title: Onboarding a GCP Autopilot shop
description: One documented path from a clean workstation to a working Magento shop on the GCP Autopilot preview recipe.
---

# Onboarding a GCP Autopilot shop

One path from a clean workstation to a working shop on the alpha recipe:
GCP GKE Autopilot, Magento 2.4.9, Cloud SQL MySQL 8.4, Valkey 9.0,
OpenSearch on GKE, database queue, GCS media, HTTPS load balancer.
Follow it in order; every step names its command and what good looks
like. Anything needing a human is in Prerequisites, not discovered
mid-deploy.

## Prerequisites

Do these first. MageLift automates what it honestly can; the rest
needs you (or your registrar, Google billing, Adobe account team).

- **GCP account with billing activated.** Console: Billing, link a
  payment method to the project. There is no automated billing
  setup, and no deploy works without it.
- **Operator credentials.** Either `gcloud auth login` on your
  workstation (developer path) or workload identity federation for
  CI. `magelift doctor` reports whether `gcloud` is present; it is
  guidance, not a gate, because WIF needs no CLI.
- **CI service account API grants (CI only).** `magelift bootstrap`
  mints the workload identity pool, provider, and service account
  plus the repository impersonation binding. Granting the service
  account API roles on your project stays an operator IAM step;
  cover at least the services admission checks (compute, container,
  sqladmin, secretmanager, storage, serviceusage, plus billing
  read and logging/monitoring for observability).
- **Domain ownership and DNS.** The zone and its records stay
  operator-managed: nothing in the stack creates DNS. After deploy,
  point your names at the load balancer address from
  `magelift outputs` (see Verify).
- **Adobe Commerce license and Composer auth.** Commerce edition
  needs your license; builds need Composer credentials as a
  `gcp-secret-manager://` reference. Put the reference in YAML,
  never the value.
- **SMTP relay account.** Alpha sending is operator relay: any
  SMTP relay with host, port, username, and a password stored as
  a `gcp-secret-manager://projects/.../secrets/.../versions/...`
  value. Create the relay account and the secret yourself, then
  set the `email` block (the starter carries a commented example).
  Sandbox and sending-limit exits on the relay side stay manual.

What MageLift does automate, so nobody does it by hand: database
and queue credentials are generated and stored, secrets travel by
reference into Kubernetes Secrets consumed by `SecretKeyRef` only,
and TLS terminates at the managed load balancer.

## Steps

### 1. Install

```sh
go install github.com/magelift/magelift/cmd/magelift@latest
magelift version
```

Release archives replace `go install` after the first tag; see
[Install](install.md).

### 2. Template

```sh
magelift init --provider gcp
```

Fill in your GCP project and domains in `magelift.yaml`. The
starter matches the alpha recipe (preview preset, baked static
content, recipe search); staging and production inherit with
larger presets.

### 3. Validate

```sh
magelift doctor
magelift config validate --env preview
```

`doctor` checks the config plus host tooling and prints the next
action. `config validate` accepts structural presence; the provider
validates target semantics at plan time and names the field.

### 4. Bootstrap

```sh
magelift bootstrap --env preview
```

Prepares the GCP project: state bucket with versioning, WIF pool,
provider, and service account. GCP needs no access-log bucket
flag (that one is AWS-only).

### 5. Deploy

```sh
magelift deploy --env preview --yes
```

Preview deploys email-disabled unless you set the `email` block.
Every failure carries a typed cause with a next step; see
When it fails.

### 6. Verify, per surface

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
- **Search:** reindex, then query the storefront:
  `magelift --env preview exec --service web -- bin/magento indexer:reindex catalogsearch_fulltext`,
  then `curl -s 'https://preview.example.com/catalogsearch/result/?q=bag'`
  returns results HTML.
- **Cron/consumers:** `magelift --env preview logs --service cron`
  shows recent scheduler runs; the DB queue needs no broker health
  cell on preview.
- **SMTP:** create an admin user, then trigger the storefront
  forgot-password flow for it and confirm arrival at the relay:
  `magelift --env preview exec --service web -- bin/magento admin:user:create --admin-user=owner --admin-password=<secret> --admin-email=owner@example.com --admin-firstname=Shop --admin-lastname=Owner`.
  Never print relay credentials; check the relay's own logs.
- **Media:** upload a product image in admin, then confirm the
  object in the `mediaBucket` from outputs
  (`gcloud storage ls gs://<mediaBucket>/`).
- **Cost:** `magelift --env preview cost` prints account-free
  capacity with unpriced items listed; `--live` is not wired on
  GCP and errors rather than guessing. Budgets alert, never cap.

### 7. Teardown

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
