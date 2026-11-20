# Frequently asked questions

## Who hosts the store?

The store runs in the shop's cloud account. Your team controls the image
registry, infrastructure state and shop data. MageLift uses the selected
provider plugin to manage the deployment. The cloud provider bills your
account for the resources the shop uses.

## Which Magento releases and services can I select?

Declare the exact Magento release and the PHP and service versions you need. The CLI reads Composer metadata when you initialize a shop. Its Adobe compatibility snapshot supplies recommendations and warnings. Actual provisioning depends on the adapter, region and the cloud API. Teams can connect existing services, use compatible runtime images from their own registry or extend a provider. [Provider capabilities](capability-matrix.md) · [Configuration](configuration.md)

## What does MageLift test?

MageLift publishes the outcomes of specific tests. Pulumi mocks inspect resource graphs. Floci tests cover individual SDK calls. A real Magento installation exercises its application and dependencies. Use the scope, versions and dates of each result when deciding what to accept for production. [Evidence](evidence/README.md)

## How do I keep testing environments close to production?

Define the application and service versions once, then set the capacity preset
per environment. `magelift env diff staging production` shows where they differ.
The starter protects production from teardown. Check which services can fail
over and rehearse a restore before relying on the high-availability preset.

## Can I bring an existing project?

Run `magelift init` from the shop checkout. For Adobe Commerce Cloud, Upsun or a project with Compose, select the source importer and choose AWS, GCP or Azure. Review the generated configuration and any unmapped keys, then rehearse database, media and traffic cutover. [Migration guide](migrating-from-paas.md)

## Can a shop use a headless frontend?

Run Magento as the backend and connect the separately hosted frontend using the Magento GraphQL endpoint. Configure allowed storefront origins per environment. [Storefront recipes](storefront-recipes.md)
