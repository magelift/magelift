# Start a Magento project

Install the CLI from the MageLift checkout with `go install ./cmd/magelift`. The CLI evaluates the shop’s project file with PHP. Local development also needs Docker Engine, Compose and Buildx. Run `magelift doctor --local` to inspect the workstation before connecting a cloud account.

From a Magento repository containing `composer.json`, `composer.lock` and `bin/magento`, run the following commands. Choose `aws`, `gcp` or `azure` as the destination when initializing the project.

```sh
magelift init --provider aws
magelift config validate
magelift env diff staging production
magelift dev
```

Open the URL printed by `dev`. It starts the Compose services, installs Magento into an empty database, and queries Magento’s store configuration over HTTP. On the first install it saves the generated administrator password in `.magelift/local.env`. Keep this file private. `magelift local down` stops the containers and preserves the shop’s volumes.

For a new shop, run `magelift init --new --provider gcp` in an empty Git repository. Composer downloads Magento Open Source from Adobe’s repository using your Marketplace credentials. For licensed Adobe Commerce use `magelift init --new --edition commerce` with an entitled Marketplace account. Add `--magento-version 2.4.8-p1` to request an exact release. Existing Commerce projects use the edition and version in their Composer lock. For a shop hosted on Adobe Commerce Cloud, Upsun Flex or Platform.sh, use the importer. [Migration guide](migrating-from-paas.md)

To fill the destination during initialization, pass `--cloud-account` (AWS account ID, GCP project ID or Azure subscription ID), `--region`, and optionally `--domain`. For example, `--domain store.example.com` selects that production host, `uat.store.example.com` for UAT, and a separate hostname for every preview. The Azure starter uses a load balancer address. To use your domain, configure an ingress controller and TLS. Production’s HA preset also needs a highly available external cache reachable from AKS. [Azure setup](bootstrap.md#azure-aks)

## Configure the shop

Commit `magelift.php` and any generated `magelift.environments.php`. An environment added by the CLI lives in the companion file so an edit can preserve the PHP you authored. The evaluator provides the SDK classes. `init` also installs the class definitions under `.magelift/` for editor completion. Treat project files and required PHP helpers as executable code from the repository.

The starter defines staging, UAT, production and preview. Staging and previews use the smaller preset. UAT starts at standard capacity and production uses the protected high-availability preset. Define a dependency once at project level and override capacity or an individual choice in an environment. Inspect the result before deployment. [Choosing service versions and ownership](service-bindings.md)

```sh
magelift --env production config effective
magelift --env staging config explain target.provider
magelift env diff staging uat
```

## Connect the cloud

Cloud commands load the signed plugin for the selected account. You can prepare it ahead of time with `magelift providers install`. [Provider installation](install.md#providers)

Replace the starter’s example account, project, subscription, region and domain with yours. Add the provider-specific identity, state, encryption and artifact registry settings. Keep passwords in your cloud secret manager. The release image belongs in a registry controlled by the shop. Use your own image build or the optional MageLift build recipes. [Bootstrap](bootstrap.md) · [Builds](builds.md) · [Provider coverage](capability-matrix.md)

Generate the GitHub Actions workflow with `magelift ci generate --magelift-version vX.Y.Z`, replacing the version with an available immutable CLI release. Inspect the workflow and configure per-environment identities and GitHub environment protection. After reviewing the current head of a same-repository pull request to `main`, apply `magelift-preview`. Each new head needs a fresh label event. The build uses the reviewed head and the base revision’s PHP configuration. Use protected GitHub environments for both `preview-build` and `preview`. Removing the label or closing the PR schedules teardown. The hourly sweep cleans up expired previews. Set production approvals in GitHub before enabling production promotion.

Start a headless shop with `magelift init --storefront-origin https://store.example.com`. For an existing project set `application.mode` to `headless`, configure the storefront origin and allowed CORS origins, then point your independently deployed frontend at the Magento endpoint. Keep `integrated` for Magento’s native storefront. [Storefront recipes](storefront-recipes.md)
