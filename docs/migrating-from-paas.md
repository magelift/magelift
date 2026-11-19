# Move a Magento shop to MageLift

Start in the shop's Git checkout with its committed `composer.lock`. Choose the destination independently of the old host. The importer writes `magelift.php` with staging, UAT, production and preview environments. You can inspect the project and compare environments before creating a cloud resource.

| Source | Files read | Example |
| --- | --- | --- |
| Upsun Fixed or Platform.sh | `.platform.app.yaml` or `.platform/applications.yaml`, services and routes in `.platform/`, and `.platform.env.yaml` | `magelift init --from-upsun --provider aws` |
| Upsun Flex | Named applications, services and routes in `.upsun/*.yaml` | `magelift init --from-upsun --app shop --provider gcp` |
| Adobe Commerce Cloud | `.magento.app.yaml`, `.magento/services.yaml`, `.magento/routes.yaml`, and `.magento.env.yaml` | `magelift init --from-acc --provider azure` |
| Docker Compose | Service image tags in the first standard Compose file in the shop root | `magelift init --from-compose --provider azure` |

For an Upsun shop that also uses ece-tools, MageLift reads the Magento app's `.magento.env.yaml` after the platform settings. Recognized values there take precedence. Use `--app` to select the Magento backend in a multi-app project. To connect a separately deployed storefront, pass `--storefront-origin https://store.example.com`. Leave it out for a native Magento storefront.

## Inspect the proposed project

This example writes a separate PHP file so you can try another provider in the same checkout. Supply the actual project, region and domain when you know them.

```sh
magelift init --from-upsun --provider gcp --cloud-account example-shop \
  --region europe-west1 --domain shop.example.com --config-out rehearsal.php
magelift --config rehearsal.php config validate
magelift --config rehearsal.php --env staging config effective
magelift --config rehearsal.php env diff staging production
```

Exit status 2 after the importer writes the project means that settings need review. Open the matching `rehearsal.unmapped.md`. It names the source file and key, so you can find the original value without copying it into the report. Check placeholders, redirects, custom mounts, shell hooks, Fastly rules, cache tiers and database reader routing. Translate the behavior your shop uses, including jobs started outside Magento cron. Keep vendor credentials and shell hook bodies out of the PHP project.

MageLift gets the Magento edition and release from Composer metadata. A Commerce or Cloud metapackage takes precedence over its Open Source dependency. When the source pins Composer `2.9.8`, MageLift imports it as `2.9.8+`. The builder can then use a patched Composer 2 and install the existing lockfile. Test shop-specific Composer plugins and provide credentials for licensed packages to the builder. [Build settings](builds.md#composer) · [Adobe system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements)

## Match the services and Magento settings

- Check both `services.cache` and `services.session`. Upsun shops often evict cache entries under pressure and keep sessions on another store with a different policy. The importer recognizes persistent stores and common relationship aliases. Inspect custom aliases in the report.
- Review database character set, collation and maximum packet size. The importer carries supported settings to RDS or self-hosted MariaDB and reports settings the selected service cannot accept. Self-hosted volumes round the source disk size up to the next whole GiB.
- Check the static content theme and locale matrix, patch IDs, consumer names and session tuning in the generated PHP. A disabled source cron consumer runner leaves dedicated workers off unless you configure them. Keep the shop's patches and distributable archives in the build context. [Build settings](builds.md) · [Adobe deployment variables](https://experienceleague.adobe.com/en/docs/commerce-on-cloud/user-guide/configure/env/stage/variables-deploy)
- Compose import recognizes numeric tags of public MySQL, MariaDB, Redis, Valkey, RabbitMQ and OpenSearch images. A private search image, a Dockerfile, credentials or custom startup settings need review. A locked Smile ElasticSuite installation adds its ICU and phonetic analyzer requirements. Check which adapter Magento selects and reindex the catalog on the destination. [Service choices](service-bindings.md)

## Carry over the encryption key before restoring data

A restored database needs the source Magento `crypt/key` to read encrypted configuration. Transfer the full value, including any key history, through your secret management process. Configure the destination reference before the first Magento migration. A newly generated key cannot decrypt old values.

| Target | Reference in the destination environment |
| --- | --- |
| AWS | Set `target.aws.encryptionKeySecretArn` to the Secrets Manager ARN. |
| GCP | Set `target.gcp.encryptionKeySecret` to the secret ID in the selected Google Cloud project. |
| Azure | Set `target.azure.encryptionKeySecret` to a versioned Key Vault secret URL, or `target.azure.encryptionKeySecretName` to an existing namespace-local Kubernetes Secret with a `key` entry. |

Use separate credentials and sanitized data in PR previews. Keep the source available while you rehearse SQL and media restore, search indexing, checkout and the traffic switch. Record how to recover writes made after cutover before you change DNS. [Rehearse the data migration](weekend-migrate.md)
