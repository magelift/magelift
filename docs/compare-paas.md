---
title: Compare MageLift to ACC / Upsun
description: MageLift vs Adobe Commerce Cloud and Upsun. Own-cloud Magento with ACC/Upsun-shaped YAML versus a rented Magento PaaS. Cost model, lock-in, certified AWS/GCP targets.
---

# Compare MageLift to ACC / Upsun

Adobe Commerce Cloud and Upsun-style products run Magento on their platform.
MageLift is free open-source software that runs Magento in your AWS or GCP
account with familiar YAML. You pay the cloud bill and control IAM.

Cost numbers here are illustrative. Plug in your own quotes.

## Audience

- Agencies with roughly 3-20 Magento shops that have (or want) AWS/GCP accounts
- SME Magento owners who can follow YAML/CLI docs without a dedicated DevOps hire

## PaaS vs MageLift

| | Adobe Commerce Cloud / Upsun-style PaaS | MageLift |
| --- | --- | --- |
| Where Magento runs | Vendor-managed platform | Your AWS or GCP account |
| Config shape | PaaS YAML / UI | ACC/Upsun-shaped `magelift.yaml` + CLI |
| Lock-in | Platform + often region/tooling | Cloud APIs + open-source CLI (Apache-2.0) |
| Cost model | Platform fee + usage | Cloud bill (and your time). `magelift cost` estimates AWS shapes |
| Software fee | Platform subscription | $0 for MageLift ([pricing](https://magelift.dev/pricing.md)) |
| Ops | Ticket / platform runbooks | Day-2 verbs on certified targets; you own IAM and spend |

MageLift keeps billing in your account, uses ACC/Upsun-shaped YAML, and ships
under Apache-2.0. Certified targets are listed in the capability matrix. Preview
stacks can be destroyed when you are done.

ACC / Upsun-style PaaS gives you a vendor-operated platform and tickets, with less
raw cloud surface for small teams. You pay a platform fee and accept platform
lock-in; Magento runs on rented infrastructure you do not fully control.

## What is production-supported

| Target | Status |
| --- | --- |
| AWS ECS Fargate | Certified |
| GCP GKE Autopilot | Certified |
| AWS EKS / OVH / Scaleway | Experimental |

Details: [capability matrix](capability-matrix.md). Experimental targets are not
production-supported.

## Cost assumptions

Use `magelift cost --env preview` for AWS catalog shapes. Typical preview docs
assume:

- A region that fits your account's free-tier story where relevant
- `searchMode: disabled` or other free-tier-safe search
- Database queues on preview (`queueMode: db`); prefer `ecs-rabbitmq` when you
  need a broker without Amazon MQ pricing
- Destroy when the spike is done

MageLift does not claim a fixed monthly savings figure. Compare against your PaaS
quote with the same traffic and catalog assumptions.

## Migration

Config import (`magelift init --from-acc` / `--from-upsun`), dump seed, media, and
DNS cutover: [migrating-from-paas](migrating-from-paas.md) and
[weekend-migrate](weekend-migrate.md). Existing AWS VPC/RDS:
[brownfield-attach](brownfield-attach.md).

## FAQ

### Is MageLift cheaper than Adobe Commerce Cloud?

It can be, but MageLift does not publish a fixed savings number. You replace the
platform fee with your AWS/GCP bill plus operator time. Run `magelift cost` and
compare to your PaaS quote on the same assumptions.

### Will my Magento team recognize the config?

`magelift.yaml` follows ACC/Upsun habits. Power users open the capability matrix
for explicit catalog fields.

### Can I leave PaaS in a weekend?

Packaging and checklist: [weekend-migrate](weekend-migrate.md). Full dump/DNS
cutover depth: [migrating-from-paas](migrating-from-paas.md).

## Trust

- Apache-2.0 and `NOTICE`
- First release ships archives with checksums / SBOM (GoReleaser); images use Cosign on GHCR
- Hosted CI runs on the public repo ([release-readiness](release-readiness.md));
  path-filtered green on `main` is required before `v1.0.0-rc.1`

## Next

[Getting started](getting-started.md) → `examples/sample-shop/` in the repo →
[FAQ](faq.md) →
[CONTRIBUTING](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md).
