# Compatibility

The catalog snapshot is dated 2026-07-18. MageLift accepts the PHP branches that
PHP still supports and only enables Magento releases whose Adobe requirements have
a PHP 8.2 or newer intersection.

| Magento release line | MageLift PHP branches | v1 AWS service floor |
| --- | --- | --- |
| 2.4.9 | 8.5 | Aurora MySQL 3.12, OpenSearch 3.x, Valkey 8.x or 9.x, AWS MQ RabbitMQ 3.13 or 4.2* |
| 2.4.8 | 8.3, 8.4 | Aurora MySQL 3.12, OpenSearch 3.x, Valkey 8.x, AWS MQ RabbitMQ 3.13 or 4.2* |
| 2.4.7 | 8.2, 8.3 | Aurora MySQL 3.11 or 3.12, OpenSearch 2.x or 3.x, Valkey 8.x, AWS MQ RabbitMQ 3.13 or 4.2* |
| 2.4.6 | 8.2 | Aurora MySQL 3.11 or 3.12, OpenSearch 2.x or 3.x, Valkey 8.x, AWS MQ RabbitMQ 3.13 or 4.2* |
| 2.4.5, 2.4.4 | unavailable | Adobe lists PHP 8.1 only, below MageLift's PHP 8.2 floor |

\* RabbitMQ 4.2 requires an `mq.m7g` broker instance in Amazon MQ.

These service values are the AWS forms of Adobe's current compatibility tables, not
generic Docker image tags. RabbitMQ 4.2 is restricted to `mq.m7g` broker instances
by Amazon MQ. The planner rejects a mismatch before Pulumi registers resources.
`compatibility.allowUnsupported: true` records an explicit exception in the planned
artifact metadata; it does not make the combination certified.

Sources:

- [Adobe Commerce system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements)
- [PHP supported versions](https://www.php.net/supported-versions.php)
- [Amazon MQ RabbitMQ engine versions](https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-version-management.html)
- [Amazon ElastiCache engine versions](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/engine-versions.html)
