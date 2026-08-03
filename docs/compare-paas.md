# Compare MageLift to ACC / Upsun

Honest evaluator page  - not a sales sheet. Numbers are **illustrative**; replace
with your quotes before a purchase decision.

## Who this is for

- Agencies running 3-20 Magento shops that already have (or want) their own cloud
  accounts
- SME Magento owners without a dedicated DevOps hire who can follow YAML/CLI docs

## What you buy vs what you rent

| | Adobe Commerce Cloud / Upsun-style PaaS | MageLift |
| --- | --- | --- |
| Where Magento runs | Vendor-managed platform | **Your** AWS or GCP account |
| Config shape | PaaS YAML / UI | ACC/Upsun-shaped `magelift.yaml` + CLI |
| Lock-in | Platform + often region/tooling | Cloud APIs + open-source CLI (Apache-2.0) |
| Cost model | Platform fee + usage | Cloud bill (+ your time). CLI has `magelift cost` estimates for AWS shapes |
| Ops | Ticket / platform runbooks | Day-2 verbs on certified targets; you own IAM and spend |

## Certified targets (do not oversell)

| Target | Status |
| --- | --- |
| AWS ECS Fargate | **Certified** |
| GCP GKE Autopilot | **Certified** |
| AWS EKS / OVH / Scaleway | **Experimental** |

Honesty rules: [capability matrix](capability-matrix.md). Experimental cells are
not production-supported.

## Cost framing (assumptions)

Use `magelift cost --env preview` for AWS catalog shapes. Typical **preview**
assumptions in docs:

- Region in a free-tier-friendly account where applicable
- `searchMode: disabled` or free-tier-safe search
- Database queues on preview (`queueMode: db`); prefer `ecs-rabbitmq` when you
  need a broker without Amazon MQ pricing
- Destroy-when-done for spikes

PaaS monthly quotes vary widely by commerce plan. MageLift does **not** claim a
fixed $/mo savings  - publish your own side-by-side with the same traffic and
catalog assumptions.

## Migration

Config import (`magelift init --from-acc` / `--from-upsun`), dump seed, media, and
DNS cutover: [migrating-from-paas.md](migrating-from-paas.md) and
[weekend-migrate.md](weekend-migrate.md). Brownfield VPC/RDS on AWS:
[brownfield-attach.md](brownfield-attach.md).

## Trust signals

- Apache-2.0 + `NOTICE`
- Release archives with checksums / SBOM (GoReleaser)
- Cosign signing path for images
- Hosted CI green is a launch gate (see [release-readiness.md](release-readiness.md);
  do not claim green while Actions minutes are deferred)

## Next

[Getting started](getting-started.md) → sample-shop YAML under `examples/sample-shop/`
in the repository → GitHub issues / PRs via
[CONTRIBUTING](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md).
