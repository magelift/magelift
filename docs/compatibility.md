# Compatibility

The Adobe catalog snapshot is dated 2026-11-02. It records Adobe's latest-patch
requirements separately from MageLift's implementation status. A catalog match
does not certify a provider or an architecture.

| Release | PHP in the bundled runtime recipes | Adobe patch in the snapshot |
| --- | --- | --- |
| 2.4.9 | 8.5 | 2.4.9 |
| 2.4.8 | 8.3, 8.4 | 2.4.8-p5 |
| 2.4.7 | 8.3 | 2.4.7-p10 |

MageLift tests local images for PHP 8.3 through 8.5. Adobe lists PHP 8.2 for 2.4.6 and PHP 8.1 for 2.4.4 and 2.4.5; those lines need an application image that supplies their PHP version. Adobe also lists PHP 8.2 for 2.4.7.

The Adobe rows include MariaDB, MySQL where listed, OpenSearch, Elasticsearch
where listed, RabbitMQ, ActiveMQ Artemis, Valkey, Varnish, nginx, Composer, and
PHP. The catalog also includes MageLift choices for database-backed messaging,
no Varnish, and Fastly edge migration. Redis is marked unsupported wherever the
latest Adobe row marks it unsupported.

Adobe lists Composer 2.10 for Magento 2.4.9 and 2.4.8-p5. Its 2.4.9 release notes also record compatibility with Composer 2.9. When revisiting an older checkout, check the shop’s locked plugins against the Composer binary you intend to run. Composer 2.11.2 fixed high-severity issues affecting earlier builds. Updating the build tool can leave the shop’s locked packages in place. [Adobe system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements) · [Adobe 2.4.9 release notes](https://experienceleague.adobe.com/en/docs/commerce-operations/release/notes/adobe-commerce/2-4-9) · [Composer 2.11.2 fixes](https://getcomposer.org/changelog/2.11.2)

Inspect the full matrix without reading generated documentation:

```sh
magelift compatibility catalog 2.4.9
magelift compatibility validate --release 2.4.8-p5 \
  --component search --option elasticsearch --version 8
magelift certification targets
magelift certification cells --target aws/ecs-fargate --release 2.4.9 \
  --edition open-source --preset preview
magelift certification seal --file .magelift/acceptance-evidence.jsonl \
  --output-file .magelift/acceptance-evidence.sealed.jsonl
magelift certification verify --file .magelift/acceptance-evidence.sealed.jsonl
```

The legacy `certification` command also reads the archived provider catalog and verifies sealed JSONL evidence. A catalog entry describes a configuration. A linked run records the image digest, operation and cleanup observed on that date.

Statuses have deliberately narrow meanings:

- `adobe-supported` appears in Adobe's current system-requirements table.
- `magelift-compatible` is a MageLift topology or migration choice that Adobe
  does not define as a dependency.
- `unsupported` is explicitly outside the current Adobe row.
- `unavailable` means the release or service cannot be used on the selected
  MageLift path.

For a recorded cloud run, check the exact provider, runtime and operation. [Provider evidence](capability-matrix.md)

Choices outside the snapshot produce warnings and remain selectable. An old snapshot does not block planning. The legacy `compatibility.allowUnsupported` setting remains accepted. Neither the setting nor a warning certifies a cell or makes an unavailable provider feature available. Local versions outside the tested rows use unpinned image tags and are identified in the plan.

Sources:

- [Adobe Commerce system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements?lang=en)
- [Adobe search engine prerequisites](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/prerequisites/search-engine/overview)
- [PHP supported versions](https://www.php.net/supported-versions.php)
- [Amazon MQ RabbitMQ engine versions](https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-version-management.html)
- [Amazon ElastiCache engine versions](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/engine-versions.html)
