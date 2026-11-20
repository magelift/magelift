# Sample Magento shop configuration

This directory contains an AWS ECS Fargate PHP project and a generated GitHub
Actions workflow. Copy the project into a Magento shop with its Composer lock,
or run `magelift init --provider aws`, `--provider gcp` or `--provider azure`
from the shop root. A new starter includes staging, UAT, production and preview.
The sample has preview, staging and production. Add UAT if your shop needs it.

Review the project name, cloud account, region, domains, service selections and
secret references before use. The account IDs and domains in the sample are
placeholders. Keep Composer credentials and Magento encryption keys in secret
stores, not in `magelift.php`. Review the PHP file as code before running it.

For local development, install the shop's Composer dependencies and run:

```sh
magelift doctor --local
magelift dev
```

For a cloud environment, follow [getting started](../../docs/getting-started.md)
and bootstrap the target. AWS bootstrap also needs an existing access-log bucket.
Build and push an image to your registry before deploying its immutable digest.
See [operations](../../docs/operations.md) for health checks and teardown.

## Pull-request previews

Generate the workflow with an available signed MageLift release and review any
candidate file before replacing a customized workflow:

```sh
magelift ci generate --magelift-version v1.2.3
magelift ci validate --magelift-version v1.2.3
```

The version above is an example. Pull requests run credential-free validation.
A reviewer can apply `magelift-preview` to a same-repository PR targeting `main`
to build its current commit. The build can access configured build credentials,
so apply the label only to trusted code. Removing it requests preview cleanup.
Review each new commit, then remove and reapply the label. The previous
preview may be gone before the replacement is ready. Configure GitHub environment protection
for `build`, `preview-build`, `preview` and each stable deployment environment.

Configure the registry and provider variables for those environments. AWS uses
role ARNs. GCP uses workload identity federation with separate build and
deploy service accounts. Azure uses an Entra federated identity and requires the state
account, tenant and passphrase settings in [bootstrap](../../docs/bootstrap.md).
Do not add long-lived cloud keys to the workflow.

## Existing platform or headless storefront

For an Upsun or Adobe Commerce Cloud project, follow
[migrating from PaaS](../../docs/migrating-from-paas.md). The importer produces a
PHP configuration you can review. Transfer the database and media separately.

For an externally deployed frontend, pass `--storefront-origin` during `init` or
configure the headless application and allowed origins in PHP. Deploy the
frontend with your usual tools and connect it to Magento. See
[storefront recipes](../../docs/storefront-recipes.md).
