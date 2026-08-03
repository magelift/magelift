# Benchmarking preset capacity

Preset capacity is a measured input, not a guess. Run the benchmark against a
representative Magento installation and keep the JSON report with the release
artifacts:

```sh
magelift benchmark run \
  --url https://staging.example.com \
  --duration 10m \
  --concurrency 50 \
  --mix '/=70,/graphql=20,/customer/section/load=10' \
  --catalog magento-standard \
  --catalog-revision <commit> \
  --magento-version 2.4.9 \
  --php-version 8.5 \
  --database 'Aurora MySQL 3.12' \
  --search 'OpenSearch 3.x' \
  --queue 'RabbitMQ 4.2' \
  --cache 'Valkey 8.x' \
  --output json > benchmark.json
```

The report records the catalog identity, Magento and service versions, request
mix, concurrency, sample count, successful and failed requests, p50/p95/p99
latency, throughput, and the cost inputs needed for regional pricing. It does not invent AWS prices;
`cost.status` is `unpriced` until the same catalog has been evaluated with
current regional pricing and workload measurements.

Use a fixed catalog revision for comparisons. The catalog should document
Magento edition and version, PHP and service versions, fixture size, request
mix, concurrency schedule, warm-up policy, cache state, failure injection, and
the region and pricing date used for any cost estimate. Do not compare reports
from different catalogs as if they were the same workload.

The runner is an HTTP measurement tool, not a Magento correctness test. Run
the application lifecycle and smoke tests separately, and treat failed or
partial benchmark reports as unsuitable for changing preset defaults.
