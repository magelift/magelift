---
title: FAQ
description: Common questions about MageLift: open-source Magento CLI for AWS and GCP, pricing, certified clouds, ACC/Upsun migration, and how it differs from Magento PaaS.
---

# FAQ

For the product matrix see [capability matrix](capability-matrix.md). For PaaS
comparison see [compare to ACC / Upsun](compare-paas.md).

## What is MageLift?

MageLift is an open-source CLI and ACC/Upsun-shaped YAML format for Magento Open
Source and Adobe Commerce in your AWS or GCP account. You run it against your own
cloud; MageLift does not host stores.

## Is MageLift a Magento hosting provider?

MageLift does not host Magento. It is Apache-2.0 software you run in your account.
Adobe Commerce Cloud, Upsun, and similar products rent you a platform; MageLift
targets infrastructure you already control.

## How much does MageLift cost?

The CLI is free under Apache-2.0. AWS or GCP bills you for compute, data, and
network. Use `magelift cost` for AWS shape estimates and destroy preview stacks
when you are done. See [pricing.md](https://magelift.dev/pricing.md).

## Which clouds are production-supported?

AWS ECS Fargate and GCP GKE Autopilot are certified. AWS EKS, OVH MKS, and
Scaleway Kapsule are experimental and not production-supported until the matrix
marks them certified.

## Can I migrate from Adobe Commerce Cloud or Upsun?

Config import is `magelift init --from-acc` / `--from-upsun`. Dump, media, and
DNS cutover live in [migrating from PaaS](migrating-from-paas.md) and
[leave PaaS in a weekend](weekend-migrate.md).

## Does MageLift replace ece-tools?

No. See [ece-tools parity](ece-parity.md) for intentional gaps. The goal is
Magento-shaped operations in your cloud, not byte-for-byte Adobe tooling parity.

## Is MageLift affiliated with Adobe?

MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe
trademarks, used only to describe compatibility.

## How do I install MageLift?

Until the first public tag (`v1.0.0-rc.1`), use `go install`. After releases
exist, prefer GitHub Release archives (checksums and SBOM). Steps:
[Install](install.md).

## Where should I start?

1. [Install](install.md) the CLI
2. [Getting started](getting-started.md) with the sample shop
3. Optional: stay offline with [local vs cloud](local-vs-cloud.md)

<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "FAQPage",
  "mainEntity": [
    {
      "@type": "Question",
      "name": "What is MageLift?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "MageLift is an open-source CLI and ACC/Upsun-shaped YAML format for Magento Open Source and Adobe Commerce in your AWS or GCP account. You run it against your own cloud; MageLift does not host stores."
      }
    },
    {
      "@type": "Question",
      "name": "Is MageLift a Magento hosting provider?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "MageLift does not host Magento. It is Apache-2.0 software you run in your account. Adobe Commerce Cloud, Upsun, and similar products rent you a platform; MageLift targets infrastructure you already control."
      }
    },
    {
      "@type": "Question",
      "name": "How much does MageLift cost?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "The MageLift CLI is free under Apache-2.0. You pay AWS or GCP for compute, data, and network. Use magelift cost for AWS shape estimates and destroy preview stacks when finished."
      }
    },
    {
      "@type": "Question",
      "name": "Which clouds are production-supported by MageLift?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "Certified today: AWS ECS Fargate and GCP GKE Autopilot. AWS EKS, OVH MKS, and Scaleway Kapsule are experimental and not production-supported until the capability matrix marks them certified."
      }
    },
    {
      "@type": "Question",
      "name": "Can I migrate from Adobe Commerce Cloud or Upsun to MageLift?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "Config import is magelift init --from-acc or --from-upsun. Dump, media, and DNS cutover guidance are documented in the MageLift migration guides."
      }
    },
    {
      "@type": "Question",
      "name": "Is MageLift affiliated with Adobe?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe trademarks, used only to describe compatibility."
      }
    }
  ]
}
</script>
