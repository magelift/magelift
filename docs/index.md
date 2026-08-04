---
title: MageLift docs
description: Docs for MageLift, an open-source Magento CLI and YAML for AWS ECS Fargate and GCP GKE Autopilot. Install, migrate from ACC/Upsun, capability matrix.
hide:
  - toc
---

<div class="mdx-hero" markdown>
<img src="assets/logo.png" alt="MageLift logo" />

<div markdown>
<h1 class="mdx-hero__title">Run Magento in your own AWS or GCP account.</h1>

<p class="mdx-hero__lede">Open-source CLI with ACC/Upsun-shaped YAML. The store runs in your account, on your invoice. MageLift does not host stores; you pay AWS or GCP directly.</p>

<div class="mdx-hero__cta" markdown>
[Install the CLI](install.md){ .md-button .md-button--primary }
[Get started](getting-started.md){ .md-button }
</div>
</div>
</div>

Pricing: [the CLI is free](https://magelift.dev/pricing.md); cloud spend is
yours. First public tag target: `v1.0.0-rc.1` ([versioning](versioning.md)).

## Pick a path

<div class="grid cards" markdown>

-   **Install and first preview**

    ---

    Get the CLI, validate the sample shop, run a preview you can destroy.

    [Install](install.md) · [Getting started](getting-started.md)

-   **Leave a PaaS**

    ---

    Import ACC or Upsun config, move data and media, cut over DNS.

    [Weekend migrate](weekend-migrate.md) · [Full migration](migrating-from-paas.md)

-   **Operate day-2**

    ---

    Deploys, secrets, cost checks, and protected destroys on certified targets.

    [Operations](operations.md) · [CLI reference](cli-reference.md)

-   **Check what is supported**

    ---

    Certified targets are the only production paths; the matrix says why.

    [Capability matrix](capability-matrix.md) · [Compare PaaS](compare-paas.md)

-   **Stay offline first**

    ---

    Local targets for exploration before any cloud spend.

    [Local vs cloud](local-vs-cloud.md)

-   **Extend MageLift**

    ---

    Add a cloud target through the provider boundary.

    [Adding a provider](adding-a-provider.md)

</div>

## Certified vs experimental

!!! success "Certified for production"
    AWS ECS Fargate, GCP GKE Autopilot.

!!! warning "Experimental"
    AWS EKS, OVH MKS, Scaleway Kapsule. Exploration only until the matrix marks
    them certified.

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
