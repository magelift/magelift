---
title: FAQ
description: Common questions about MageLift: open-source Magento CLI for AWS, GCP, Scaleway, and OVHcloud, pricing, certified clouds, ACC/Upsun migration, and how it differs from Magento PaaS.
---

# FAQ

For the product matrix see [capability matrix](capability-matrix.md). For PaaS
comparison see [compare to ACC / Upsun](compare-paas.md).

## What is MageLift?

MageLift is an open-source CLI and ACC/Upsun-shaped YAML format for Magento Open
Source and Adobe Commerce in your AWS, GCP, Scaleway, or OVHcloud account. You run
it against your own cloud; MageLift does not host stores.

## Is MageLift a Magento hosting provider?

MageLift does not host Magento. It is Apache-2.0 software you run in your account.
Adobe Commerce Cloud, Upsun, and similar products rent you a platform; MageLift
targets infrastructure you already control.

## How much does MageLift cost?

The CLI is free under Apache-2.0. Your selected cloud provider bills you for
compute, data, and network. Use the provider-specific cost and certification
guidance in the [capability matrix](capability-matrix.md), and destroy preview
stacks when you are done. See [pricing.md](https://magelift.dev/pricing.md).

## Which clouds are production-supported?

AWS ECS Fargate and the evidenced GCP GKE Autopilot cells are certified. AWS
EKS, GCP GKE Standard, OVH MKS, and Scaleway Kapsule are experimental and not
production-supported until the matrix marks their target cells certified. Native
edge, Fastly, native observability, and New Relic claims are independently
evidence-gated from runtime certification.

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

## What is the Magento web runtime?

`nginx-fpm` is the Adobe-aligned default. `frankenphp-classic` and `php-apache`
are Adobe-unsupported plugins that require `compatibility.allowUnsupported` on
the current Magento line. Adobe has no row for either plugin, neither is
certified, and `frankenphp-worker` is unregistered.

## What is the difference between `magelift audit` and `magelift evidence`?

`magelift evidence` is the production change journal: digest, actor, config
provenance, backup policy. `magelift audit` lists encryption, IAM, logging,
backup, WAF, and residency pointers from YAML. Neither is a SOC 2, ISO 27001,
or GDPR certificate. MageLift is software you run in your account; those
attestations belong to your organization and cloud provider.

## Can I declare Magento websites in YAML?

No. `application.magento` overlays `frontName`, cookies, CORS, consumers, and
optional SQS/Pub/Sub module transports. Websites, stores, and store views stay
in Magento.

## Which platform gaps remain?

MageLift has no `websites[]` YAML or Cloud SQL attach. SQS and Pub/Sub need
Magento modules and locked Composer packages. Split Valkey cache and session
endpoints are AWS-only; GCP, OVH, and Scaleway use one cache endpoint. OVH
native CDN fails closed, and Scaleway provides Redis rather than Valkey.
GCP Cloud Armor Magento `requestBodiesToExclude` is withheld. Autopilot cannot
set `vm.max_map_count` for three-node OpenSearch, so HA OpenSearch stays on
GKE Standard and does not inherit Autopilot certified.

## Is SQS or Pub/Sub a queue catalog cell?

No. Certified AWS queue cells are `db` and `ecs-rabbitmq`. SQS and Pub/Sub
require `application.magento.queueTransport` plus a Magento Composer package in
`composer.lock`. MageLift does not provision those brokers as catalog cells.

## Does `compatibility.allowUnsupported` make an experimental cell certified?

No. That flag is the Adobe-unsupported hatch only. MageLift-experimental cells
warn and continue. Provider-unavailable cells still fail closed.

## Are CloudWatch logs the same as X-Ray?

No. CloudWatch logs are a separate AWS capability. X-Ray Magento traces are
typed unsupported until an ObservabilityAdapter plugin is registered; YAML
`nativeProvider: xray` is typed unavailable. An EKS IAM snippet is not Magento
X-Ray evidence.

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
        "text": "MageLift is an open-source CLI and ACC/Upsun-shaped YAML format for Magento Open Source and Adobe Commerce in your AWS, GCP, Scaleway, or OVHcloud account. You run it against your own cloud; MageLift does not host stores."
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
        "text": "The MageLift CLI is free under Apache-2.0. You pay the selected cloud provider for compute, data, and network. Use the capability matrix for provider-specific cost guidance and destroy preview stacks when finished."
      }
    },
    {
      "@type": "Question",
      "name": "Which clouds are production-supported by MageLift?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "Certified today: AWS ECS Fargate and the evidenced GCP GKE Autopilot cells. AWS EKS, GCP GKE Standard, OVH MKS, and Scaleway Kapsule are experimental and not production-supported until the capability matrix marks their target cells certified."
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
    },
    {
      "@type": "Question",
      "name": "What is the Magento web runtime?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "nginx-fpm is the Adobe-aligned default. frankenphp-classic and php-apache are Adobe-unsupported plugins that require compatibility.allowUnsupported on the current Magento line. Adobe has no row for either plugin, neither is certified, and frankenphp-worker is unregistered."
      }
    },
    {
      "@type": "Question",
      "name": "Does compatibility.allowUnsupported make an experimental cell certified?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "No. That flag is the Adobe-unsupported hatch only. MageLift-experimental cells warn and continue. Provider-unavailable cells still fail closed. magelift audit is not a SOC 2, ISO 27001, or GDPR certificate."
      }
    }
  ]
}
</script>
