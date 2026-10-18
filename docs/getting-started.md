---
title: Getting started with MageLift
description: Install the MageLift CLI from source, configure a shop, build its image, and prepare a GCP community alpha preview.
---

# Getting started

Install the CLI, fill in `magelift.yaml`, then let Magelift prepare the cloud
account and deploy Magento. You should not need Pulumi, Kubernetes, or image
signing commands on this path.

The community alpha starts with the GCP GKE Autopilot recipe. Its current
release gates and untested paths are listed in [release readiness](release-readiness.md).
Historical certification evidence is linked from the capability matrix.

## How to get Magento running with MageLift

1. **Install the CLI.** Follow [Install](install.md), then run `magelift version`.
2. **Choose local or cloud.** See [local vs cloud](local-vs-cloud.md). Local Compose needs no cloud credentials.
3. **Start a config.** Run `magelift init --provider gcp`, fill the placeholders, then `magelift doctor`.
4. **Build, then preview.** Follow [application builds](builds.md), then preview the GCP recipe; destroy disposable resources when done.
5. **Optional migrate.** Leave ACC/Upsun via [weekend-migrate](weekend-migrate.md).

## 1. Install

No release is published yet. Install from source, as described in
[Install](install.md):

```sh
go install github.com/magelift/magelift/cmd/magelift@main
magelift version
```

Work from your shop repository. The CLI carries its build recipes, so the
shop checkout does not need the MageLift source tree.

## 2. Local vs cloud

Read [local vs cloud](local-vs-cloud.md). Local Compose is for Magento lifecycle
without cloud credentials; it is not a production replica.

```sh
magelift local init
magelift local up
```

## 3. Starter config

Generate a GCP starter, then fill the project, domains and secret references.
Each starter includes preview, staging and production environment blocks:

```sh
magelift init --provider gcp    # GCP GKE Autopilot; community alpha path
# magelift init --provider aws  # AWS ECS Fargate; secondary path
```

What you get: preview runs database-backed queues (`db` on AWS,
`database` on GCP); GCP serves recipe OpenSearch unless you pin it
off for a first smoke run. Staging and production inherit with
larger presets. `magelift config effective --env preview` shows
every resolved value, so no default stays hidden. New GCP teams
should follow [GCP onboarding](onboarding.md) instead of assembling
these steps ad hoc.

Fill the placeholders (accounts or the GCP project, domains, secret
refs; no plaintext secrets), then:

```sh
magelift doctor
```

Doctor validates YAML and prints the next Magelift command. Stay on that
command. The full worked example lives at
[`examples/sample-shop/magelift.yaml`](https://github.com/magelift/magelift/tree/main/examples/sample-shop)
with its "What to replace" table in `examples/sample-shop/README.md`.
Then continue with [local vs cloud](local-vs-cloud.md) if you are still
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
magelift ci generate --magelift-version v1.2.3
magelift ci validate --magelift-version v1.2.3
```

`v1.2.3` is a placeholder. `ci generate` requires an immutable release
version, and no release is published yet. Until a tag exists, install the
CLI in CI with `go install github.com/magelift/magelift/cmd/magelift@main`.

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
