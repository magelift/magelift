# Move a Magento shop from Upsun or Adobe Commerce Cloud

Work in a checkout of the source repository. Keep the original configuration, a usable source backup, and the current environment running through the migration rehearsal.

| Source | Configuration MageLift reads | Example |
| --- | --- | --- |
| Adobe Commerce Cloud | `.magento.app.yaml`, `.magento/services.yaml`, `.magento/routes.yaml`, `.magento.env.yaml` | `magelift init --from-acc --provider aws` |
| Upsun Flex | Named applications, services and routes in `.upsun/*.yaml` | `magelift init --from-upsun --app shop --provider azure` |
| Upsun Fixed / Platform.sh | `.platform.app.yaml` or `.platform/applications.yaml`, services and routes in `.platform/` | `magelift init --from-upsun --provider gcp` |

On Upsun Fixed, MageLift looks for environment settings in `.platform.env.yaml`. If your deployment also runs ece-tools, review `.magento.env.yaml` separately. The unmapped report covers only files the importer reads. Check static content, patches, consumer schedules, database routing and session settings before accepting the generated project. [Adobe deployment variables](https://experienceleague.adobe.com/en/docs/commerce-on-cloud/user-guide/configure/env/stage/variables-deploy)

Choose the destination provider independently of the source. When a repository has multiple apps, select the Magento application by name. The importer inspects its Composer metadata to determine the edition and exact release. An Enterprise or Cloud metapackage takes precedence when the lock also contains the Community metapackage.

The generated PHP includes recognized PHP and Composer settings, Magento environment variables, static content, service engine versions and routes. For an unsupported setting the importer writes a review file next to the project file and exits with status 2. The report contains source paths and key names. Review shell hooks, custom services, per-service options, and application mounts before deploying. Store secret values separately, including the original Magento encryption key needed to decrypt imported data.

Compare how each service is used as well as its version. A cache and a persistent session store can use the same engine with different eviction policies. OpenSearch plugins, queue consumers and CDN purge rules also affect storefront behavior. Supply a hostname you control when the source routes contain platform placeholders.

```sh
magelift init --from-upsun --app shop --provider gcp --config-out rehearsal.php
magelift --config rehearsal.php config validate
magelift --config rehearsal.php env diff staging production
```

1. Edit the project’s real domain, cloud identifiers, registry and secret references. Select a provider-managed engine, a self-hosted implementation or an existing service with an appropriate connection adapter. A source engine/version stays visible even when the destination cloud needs a different implementation.
2. Create a staging rehearsal. Restore a consistent SQL export and a matching media snapshot into the destination using the provider’s import and media operations. Keep production customer data and credentials out of PR previews. [Data migration and cutover](weekend-migrate.md)
3. Verify the storefront or GraphQL endpoint, Admin login, checkout, search, cron, consumers and media. Check outbound email, payment and webhook destinations before enabling them.
4. Rehearse a write freeze, final synchronization and DNS or edge cutover. Record how to return traffic to the source and how writes made after cutover will be reconciled. Keep the source until the team accepts the destination and has tested a restore.
