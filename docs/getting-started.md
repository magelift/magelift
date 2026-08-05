---
title: Getting started with MageLift
description: Install MageLift, validate sample Magento YAML, run local Compose or a disposable AWS/GCP preview on certified ECS Fargate or GKE Autopilot.
---

# Getting started

Install the CLI, validate a sample config, then run locally or spin up a
disposable cloud preview.

**Target:** under 30 minutes from clone to a validated `magelift.yaml` (local path),
or about an hour to a cloud preview URL after bootstrap (AWS or GCP).

## How to get Magento running with MageLift

1. **Install the CLI.** Follow [Install](install.md), then run `magelift version`.
2. **Choose local or cloud.** See [local vs cloud](local-vs-cloud.md). Local Compose needs no cloud credentials.
3. **Copy the sample shop.** Use `examples/sample-shop/magelift.yaml`, fill account and secret refs, then `magelift config validate --env preview`.
4. **Preview on a certified target.** AWS ECS Fargate or GCP GKE Autopilot; destroy when done.
5. **Optional migrate.** Leave ACC/Upsun via [weekend-migrate](weekend-migrate.md).

## 1. Install

Follow [Install](install.md) (release archive preferred after the first tag, or
`go install`). Then:

```sh
magelift version
```

## 2. Local vs cloud

Read [local vs cloud](local-vs-cloud.md). Local Compose is for Magento lifecycle
without cloud credentials; it is not a production replica.

```sh
magelift dev init
magelift dev up
```

## 3. Sample shop config

Copy [`examples/sample-shop/magelift.yaml`](https://github.com/magelift/magelift/tree/main/examples/sample-shop) from the repository into your Magento
repo root. Complete the "What to replace" table in
`examples/sample-shop/README.md` (accounts, domains, secret refs; no plaintext
secrets).

```sh
magelift doctor
magelift config validate --env preview
```

Then continue with [local vs cloud](local-vs-cloud.md) if you are still offline,
or jump to AWS preview below.

## 4. AWS preview (certified)

Requires AWS credentials and an access-log bucket. Preview stacks are **billable**
in your account: destroy when you are done (`magelift destroy --env preview --yes`).
Prefer free-tier-safe shapes (`searchMode: disabled`, database queues) unless you
intentionally want OpenSearch or a broker. Rough AWS shape estimates:
`magelift cost --env preview`.

1. `magelift bootstrap --env preview --access-log-bucket … --github-owner … --github-repo …` ([bootstrap prerequisites](bootstrap.md))
2. Build or promote a signed image digest into config
3. `magelift preview --env preview` then `magelift deploy --env preview --yes`
4. `magelift outputs` / `magelift health`
5. Destroy when done: `magelift destroy --env preview --yes`

## 4b. GCP preview (certified)

GCP GKE Autopilot is also **certified**. Stacks are billable in your GCP project:
destroy when finished. Set a disposable project ID via env (never commit real IDs):

```sh
export MAGELIFT_GCP_PROJECT='your-disposable-project'
```

1. Complete WIF / bootstrap for the project (see [gcp-acceptance](gcp-acceptance.md)
   for the operator harness shape, or your own `gcloud` WIF setup)
2. Put a pullable Magento image digest in config
3. `magelift preview --env preview` then `magelift deploy --env preview --yes`
4. `magelift outputs` / `magelift health`
5. `magelift destroy --env preview --yes`

Maintainer create-once proof and harness details:
[gcp-acceptance.md](gcp-acceptance.md). Cost estimates via `magelift cost` are
AWS-oriented today; treat GCP spend as project billing in Cloud Console.

## 5. Leaving PaaS

Config import + dump + DNS cutover:
[migrating from PaaS](migrating-from-paas.md) and the weekend packaging guide
[leave PaaS in a weekend](weekend-migrate.md).

## More answers

[FAQ](faq.md) · [Compare to PaaS](compare-paas.md) · [pricing](https://magelift.dev/pricing.md)

## Decision pages

| Question | Doc |
| --- | --- |
| What is production-supported? | [Capability matrix](capability-matrix.md) |
| Can we cut a public tag? | [Release readiness](release-readiness.md) |
| Why not stay on ACC/Upsun? | [Compare to PaaS](compare-paas.md) |
| Architecture boundaries | [Architecture](architecture.md), [ADRs](adr/README.md) |

Independent of Adobe Inc. Product names are descriptive only.
