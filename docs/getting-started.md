# Run and deploy your first shop

Start in the repository you use for Magento development, with `composer.json`,
`composer.lock`, and `bin/magento`. Install the MageLift CLI, PHP 8.3 or later,
and Docker with Compose and Buildx. [Install MageLift](install.md)

## Run Magento locally

Pick the cloud you plan to use (`aws`, `gcp`, or `azure`). For this local
setup, you can choose a provider before creating a cloud account.

```sh
magelift init --provider aws
magelift doctor --local
magelift dev
```

Open the URL printed by `magelift dev`. Docker runs the database and other
services. When the database is empty, MageLift installs Magento before
checking the store over HTTP. It saves the generated Admin password in
`.magelift/local.env`. Keep that file private.

Supply the shop's Composer credentials if it uses private packages. Use
`magelift local logs` to read container logs and `magelift local down`
to stop the containers. Docker keeps the data volumes for the next run.

For a new Magento repository, use `magelift init --new` with your Adobe
Marketplace credentials. Add `--edition commerce` for licensed Adobe Commerce
or `--magento-version` to request a specific release. For a shop hosted on
Adobe Commerce Cloud or Upsun, import its configuration first. [Migration guide](migrating-from-paas.md)

## Review the project settings

MageLift reads the shop's Magento edition and version from Composer and writes
`magelift.php`. The starter defines staging, UAT, production, and preview
environments. Staging and previews use the smallest preset. Production is
protected and starts with the high-availability preset.

```sh
magelift config validate --env staging
magelift env diff staging production
```

Put shared service settings at the project level. Add overrides to an
environment when it needs a different size or service. Review the PHP before
you run it. Commit `magelift.php` and the `magelift.environments.php` companion
if MageLift created it. [Project settings](configuration.md)

## Connect the cloud

Start with staging. Enter your AWS account ID, Google Cloud project ID, or
Azure subscription ID and resource group in `magelift.php`. Replace the starter
region and domain with values you control. On Azure, the starter uses a load
balancer address. A custom domain needs ingress and TLS settings.

Log in with your cloud's CLI. In a shop checkout that uses a signed release,
run `magelift providers install`. A source build needs a trusted provider binary,
lockfile and Cosign bundle for cloud commands. Follow the setup for your provider
to prepare a state backend, which records the cloud resources MageLift manages,
and an image registry.
[Install providers](install.md#providers) · [Cloud setup](bootstrap.md) · [Google Cloud walkthrough](onboarding.md)

## Build and deploy to staging

A container registry stores the Magento image. Use a registry in your account
and sign in to it from Docker. From a clean shop checkout, push an image to
that registry. The build prints the path to a manifest that records the
exact image. Pass that path to deploy.

```sh
magelift build --push --image REGISTRY/shop:candidate
magelift deploy --env staging --from-build /path/to/manifest.json
```

MageLift previews the infrastructure changes, runs Magento's database upgrade,
and checks the rollout. Open the storefront URL in the deployment summary.
If the page fails to load, run the logs command printed in the summary.
[Build options](builds.md) · [Operate a shop](operations.md)

Your cloud provider bills you for running resources, including idle staging
services. Review the plan and the provider's pricing before you deploy.
[Provider support](capability-matrix.md)

## Add pull-request previews

When your staging deployment works, generate a GitHub Actions workflow
with `magelift ci generate --magelift-version vX.Y.Z`, using a published
release tag. Set up the GitHub identities and choose reviewers for the
environments. Apply the `magelift-preview` label to a reviewed pull request
from the same repository. Remove and reapply the label after reviewing a
new commit. Closing the pull request or removing its label schedules
teardown. An hourly sweep catches expired previews. [GitHub previews and promotion](operations.md)
