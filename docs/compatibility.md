# Compatibility

The catalog snapshot is dated 2026-07-23. It records Adobe's latest-patch
requirements separately from MageLift's implementation status. A catalog match
does not certify a provider or an architecture.

| Release | PHP accepted by MageLift | Adobe latest patch |
| --- | --- | --- |
| 2.4.9 | 8.5 | 2.4.9 |
| 2.4.8 | 8.3, 8.4 | 2.4.8-p5 |
| 2.4.7 | 8.2, 8.3 | 2.4.7-p10 |
| 2.4.6 | 8.2 | 2.4.6-p15 |

The Adobe rows include MariaDB, MySQL where listed, OpenSearch, Elasticsearch
where listed, RabbitMQ, ActiveMQ Artemis, Valkey, Varnish, nginx, Composer, and
PHP. The catalog also includes MageLift choices for database-backed messaging,
no Varnish, and Fastly edge migration. Redis is marked unsupported wherever the
latest Adobe row marks it unsupported.

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

The certification commands expose the provider capability intersection and
the sealed JSONL evidence gate. A cell marked compatible in the descriptor is
still pending until a live run records an immutable artifact and a cleanup
proof with no remaining owned resources.

Statuses have deliberately narrow meanings:

- `adobe-supported` appears in Adobe's current system-requirements table.
- `magelift-compatible` is a MageLift topology or migration choice that Adobe
  does not define as a dependency.
- `unsupported` is explicitly outside the current Adobe row.
- `unavailable` means the release or service cannot be used on the selected
  MageLift path.

Provider certification is tracked in the [capability matrix](capability-matrix.md)
and acceptance evidence, not inferred from this catalog.

`compatibility.allowUnsupported: true` is the Adobe hatch only. It records that
an Adobe-unsupported combination is accepted on purpose. It does not recertify a
cell, silence a MageLift-experimental warning, or open a provider-unavailable
target.

Sources:

- [Adobe Commerce system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements?lang=en)
- [Adobe search engine prerequisites](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/prerequisites/search-engine/overview)
- [PHP supported versions](https://www.php.net/supported-versions.php)
- [Amazon MQ RabbitMQ engine versions](https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-version-management.html)
- [Amazon ElastiCache engine versions](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/engine-versions.html)
