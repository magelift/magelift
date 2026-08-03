---
title: MageLift docs
description: Docs for MageLift, an open-source Magento CLI and YAML for AWS ECS Fargate and GCP GKE Autopilot. Install, migrate from ACC/Upsun, capability matrix.
---

# MageLift docs

## What is MageLift?

MageLift is an open-source CLI and YAML config for Magento Open Source and Adobe
Commerce in **your** AWS or GCP account. Config feels like ACC / Upsun; day-2 ops
are CLI verbs on certified targets. MageLift does not host stores; you pay AWS or
GCP directly.

Site: [magelift.dev](https://magelift.dev/). Pricing: [CLI is free](https://magelift.dev/pricing.md).
First public tag target: `v1.0.0-rc.1` ([versioning](versioning.md)).

## Pick a path

| If you want to… | Start here |
| --- | --- |
| Install the CLI | [Install](install.md) |
| Run a sample end to end | [Getting started](getting-started.md) |
| Stay offline first | [Local vs cloud](local-vs-cloud.md) |
| Leave ACC / Upsun | [Weekend migrate](weekend-migrate.md) · [full migration](migrating-from-paas.md) |
| Compare to Magento PaaS | [Compare PaaS](compare-paas.md) · [FAQ](faq.md) |
| See what is production-supported | [Capability matrix](capability-matrix.md) |
| Operate day-2 | [Operations](operations.md) · [CLI reference](cli-reference.md) |
| Add another cloud | [Adding a provider](adding-a-provider.md) |

## Certified vs experimental

**Certified:** AWS ECS Fargate, GCP GKE Autopilot.

**Experimental:** AWS EKS, OVH MKS, Scaleway Kapsule. Use for exploration only
until the matrix marks them certified.

## Contribute

[CONTRIBUTING](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md) ·
[SUPPORT](https://github.com/magelift/magelift/blob/main/SUPPORT.md) ·
[Code of Conduct](https://github.com/magelift/magelift/blob/main/CODE_OF_CONDUCT.md) ·
[SECURITY](https://github.com/magelift/magelift/blob/main/SECURITY.md)

Agent skills for contributors:
[`agents/`](https://github.com/magelift/magelift/tree/main/agents) in the repo.

Plain-text site index: [llms.txt](https://magelift.dev/llms.txt),
[llms-full.txt](https://magelift.dev/llms-full.txt).

MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe
trademarks, used only to describe compatibility.
