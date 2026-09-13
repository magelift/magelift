---
title: Getting started with MageLift
description: Install MageLift, validate sample Magento YAML, run local Compose or a disposable AWS/GCP preview on certified ECS Fargate or GKE Autopilot.
---

# Getting started

Install the CLI, fill in `magelift.yaml`, then let Magelift prepare the cloud
account and deploy Magento. You should not need Pulumi, Kubernetes, or image
signing commands on this path.

**Target:** under 30 minutes from clone to a validated `magelift.yaml` (local path),
or about an hour to a cloud preview URL (AWS or GCP).

## How to get Magento running with MageLift

1. **Install the CLI.** Follow [Install](install.md), then run `magelift version`.
2. **Choose local or cloud.** See [local vs cloud](local-vs-cloud.md). Local Compose needs no cloud credentials.
3. **Copy the sample shop.** Use `examples/sample-shop/magelift.yaml`, fill account and secret refs, then `magelift doctor`.
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
magelift local init
magelift local up
```

## 3. Sample shop config

Copy [`examples/sample-shop/magelift.yaml`](https://github.com/magelift/magelift/tree/main/examples/sample-shop) from the repository into your Magento
repo root. Complete the "What to replace" table in
`examples/sample-shop/README.md` (accounts, domains, secret refs; no plaintext
secrets).

```sh
magelift doctor
```

Doctor validates YAML and prints the next Magelift command. Stay on that
command. Then continue with [local vs cloud](local-vs-cloud.md) if you are still
offline, or jump to a certified cloud preview below.

## 4. Cloud preview (certified)

Requires you to be logged in to AWS (`aws`) or GCP (`gcloud`) for the account
in `magelift.yaml`. Preview stacks are **billable** in your account: destroy when
you are done. Prefer free-tier-safe shapes (`searchMode: disabled`, database
queues) unless you intentionally want OpenSearch or a broker. Rough cost:

```sh
magelift cost --env preview
```

Laptop path (no GitHub, no extra environment variables):

```sh
magelift doctor
magelift bootstrap --env preview
magelift deploy --env preview --yes
magelift health --mode runtime
magelift destroy --env preview --yes
```

On AWS, `bootstrap` also needs an existing log bucket ([bootstrap prerequisites](bootstrap.md)):

```sh
magelift bootstrap --env preview --access-log-bucket existing-log-bucket
```

Magelift prepares the account, signs the image with the current login, and
stores stack state. GitHub flags on `bootstrap` are only for Actions CI.

For pull-request previews in CI:

```sh
magelift ci generate --magelift-version v1.0.0-rc.1
magelift ci validate --magelift-version v1.0.0-rc.1
```

Then follow [bootstrap](bootstrap.md) for the CI variables Magelift prints.

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
