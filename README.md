# MageLift

Run Magento 2 on your laptop and deploy it to your AWS, Google Cloud, or Azure
account. Give a client a preview URL or promote an image you've tested in
staging. MageLift works with Magento Open Source and licensed Adobe Commerce
(Magento Enterprise Edition).

You set the shop's service versions and environments in `magelift.php`.
MageLift builds a container image from your checkout. It previews infrastructure
changes and runs Magento's database upgrades during deployment. Use the CLI to
check the running shop and read its logs. [What MageLift does](docs/what-magelift-does.md)

## Try it locally

Install the signed CLI release.

```sh
curl -fsSL https://magelift.dev/install.sh | sh
```

In the Magento checkout, choose `aws`, `gcp`, or `azure`. This example picks AWS.

```sh
magelift init --provider aws
magelift dev
```

Open the URL printed by `magelift dev`. You can run the shop locally with PHP
and Docker before connecting a cloud account. [Other installation methods](docs/install.md) · [Getting started](docs/getting-started.md)

If you are moving a shop from Upsun, Adobe Commerce Cloud, or Docker Compose,
MageLift can import the service settings. Rehearse the database and media
transfer in staging before you switch traffic. [Migration guide](docs/migrating-from-paas.md)

## Choose a cloud

The run records cover Magento deployments on AWS ECS Fargate and Google Cloud
GKE Autopilot. Azure AKS has Pulumi mock and Floci API tests. AKS, AWS EKS and
GKE Standard are experimental. Your cloud provider bills for the resources you
deploy. [Provider support](docs/capability-matrix.md)

## Contribute

Run `make verify-community` from the repository root. Contributor runbooks
live in `.agents/skills`. The provider guide explains how to use the SDK or
propose a cloud target. [Contributor guide](CONTRIBUTING.md) · [Add a provider](docs/adding-a-provider.md)

MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe
trademarks. MageLift is distributed under the [Apache License 2.0](LICENSE).
