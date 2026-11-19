---
title: Compare MageLift with Adobe Commerce Cloud and Upsun
description: Compare hosting, configuration, costs and operational responsibilities when moving a Magento shop into your cloud account.
---

# Compare MageLift with Adobe Commerce Cloud and Upsun

With Adobe Commerce Cloud or Upsun you buy a platform to run your store. With MageLift you operate Magento in your own cloud account. That gives your team control over service versions and capacity. It also makes your team responsible for access, backups, restores, upgrades and incident response.

| Decision | Adobe Commerce Cloud or Upsun | MageLift |
| --- | --- | --- |
| Where does the shop run? | On the vendor’s platform | AWS ECS Fargate, GCP GKE Autopilot or Azure AKS with a starter project |
| How do we configure it? | The platform’s YAML files and management tools | PHP in the shop repository |
| Who handles incidents? | The vendor and your team under your support agreement | Your team, with cloud logs, metrics and MageLift’s provider-specific commands |
| What do we pay for? | Your platform agreement and usage | Cloud resources and your team’s time. The CLI is free under Apache-2.0. |

## Compare costs for the same shop

Include compute, database capacity, backups, storage, search, message brokers, traffic and retained logs. A preview can share compute and still have its own billable database. On AWS, run `magelift cost --env preview` for a starting estimate and check the unpriced items against your provider’s rates. Compare both options using the same traffic and availability requirements. [Operations and cost](operations.md) · [Pricing](https://magelift.dev/pricing.md)

## Rehearse the move

Run `magelift init --from-acc` or `magelift init --from-upsun` in the shop checkout and choose the destination with `--provider`. The importer writes the settings it can translate and names anything you need to review. Restore the database and media together in staging, then test checkout, background jobs and recovery before changing DNS. If you already own the AWS network or database, you can attach those resources. [Migration guide](migrating-from-paas.md) · [Cutover checklist](weekend-migrate.md) · [Existing AWS resources](brownfield-attach.md)

AWS and GCP have recorded Magento runs for specific configurations. Azure has Pulumi resource tests and local Floci API tests. Check what was exercised for the configuration you plan to use. [Provider evidence](capability-matrix.md)
