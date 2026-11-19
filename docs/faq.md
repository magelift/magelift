# Frequently asked questions

## Who hosts the store?

The shop owns its cloud account, registry, state and data. MageLift builds and operates the infrastructure through provider adapters. The cloud provider bills the shop for the resources it creates.

## Which Magento releases and services can I select?

Declare the exact Magento release and the PHP and service versions you need. The CLI reads Composer metadata when you initialize a shop. Its Adobe compatibility snapshot supplies recommendations and warnings. Actual provisioning depends on the adapter, region and the cloud API. Teams can connect existing services, use compatible runtime images from their own registry or extend a provider. [Provider capabilities](capability-matrix.md) · [Configuration](configuration.md)

## Does MageLift certify a deployment?

MageLift publishes the outcomes of specific tests. Pulumi mocks inspect resource graphs. Floci tests cover individual SDK calls. A real Magento installation exercises its application and dependencies. Use the scope, versions and dates of each result when deciding what to accept for production. [Evidence](evidence/README.md)

## How do I keep testing environments close to production?

Define the application and service versions once, then change the capacity preset per environment. Use `magelift env diff staging production` to review the changes. The starter enables protection on production. HA also depends on the availability options of each dependency and a tested restore procedure.

## Can I bring an existing project?

Run `magelift init` from the shop checkout. For Adobe Commerce Cloud, Upsun or a project with Compose, select the source importer and choose AWS, GCP or Azure. Review the generated configuration and any unmapped keys, then rehearse database, media and traffic cutover. [Migration guide](migrating-from-paas.md)

## Can a shop use a headless frontend?

Run Magento as the backend and connect the separately hosted frontend using the Magento GraphQL endpoint. Configure allowed storefront origins per environment. [Storefront recipes](storefront-recipes.md)
