<?php

return [
    'MAGE_MODE' => 'production',
    // Magento's CLI and frontend only load the installed application command
    // set when the deployment config carries an install date. Existing
    // database-backed environments may not retain that marker in the image;
    // resolve it from the platform-provided runtime contract instead.
    'install' => [
        'date' => '#env(MAGENTO_DC_INSTALL__DATE, "Thu, 01 Jan 1970 00:00:00 +0000")',
    ],
    // Keep every deployment-config parent present so Magento can resolve the
    // nested structure before the runtime MAGENTO_DC_* values are applied.
    // The values themselves come from the process environment at runtime.
    'db' => [
        'connection' => [
            'default' => [
                'host' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST, "127.0.0.1")',
                // Magento 2.4.9's PDO adapter rejects a separate `port` key;
                // MySQL's default 3306 is used when the host is bare. Keep
                // the platform port binding available to grant helpers, but
                // do not emit it into Magento's deployment configuration.
                'dbname' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME, "magento")',
                'username' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME, "magento")',
                'password' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD)',
                'model' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__MODEL, "mysql4")',
                'engine' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__ENGINE, "innodb")',
                'initStatements' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__INITSTATEMENTS, "SET NAMES utf8;")',
                'active' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__ACTIVE, "1")',
            ],
        ],
    ],
    'cache' => [
        'frontend' => [
            'default' => [
                'backend' => '#env(MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND, "Magento\\Framework\\Cache\\Backend\\Redis")',
                'backend_options' => [
                    'server' => '#env(MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__SERVER, "127.0.0.1")',
                    'port' => '#env(MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__PORT, "6379")',
                    'database' => '#env(MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__DATABASE, "0")',
                ],
            ],
            'page_cache' => [
                'backend' => '#env(MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND, "Magento\\Framework\\Cache\\Backend\\Redis")',
                'backend_options' => [
                    'server' => '#env(MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__SERVER, "127.0.0.1")',
                    'port' => '#env(MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__PORT, "6379")',
                    'database' => '#env(MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__DATABASE, "1")',
                    'compress_data' => '#env(MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__COMPRESS_DATA, "0")',
                ],
            ],
        ],
    ],
    'session' => [
        'save' => '#env(MAGENTO_DC_SESSION__SAVE, "files")',
        'redis' => [
            'host' => '#env(MAGENTO_DC_SESSION__REDIS_HOST, "127.0.0.1")',
            'port' => '#env(MAGENTO_DC_SESSION__REDIS_PORT, "6379")',
            'database' => '#env(MAGENTO_DC_SESSION__REDIS_DB, "2")',
        ],
    ],
    'crypt' => [
        'key' => '#env(MAGENTO_DC_CRYPT__KEY)',
    ],
    'backend' => [
        'frontName' => '#env(MAGENTO_DC_BACKEND__FRONTNAME, "admin")',
    ],
    'cron_consumers_runner' => [
        'cron_run' => '#env(MAGENTO_DC_CRON_CONSUMERS_RUNNER__CRON_RUN, "0")',
        'max_messages' => 10000,
    ],
    // Magento's catalog search settings are system configuration, not
    // deployment configuration. Keep them under system/default so
    // ScopeConfigInterface sees the same shape ece-tools writes to env.php.
    'system' => [
        'default' => [
            'catalog' => [
                'search' => [
                    'engine' => '#env(MAGENTO_DC_CATALOG__SEARCH__ENGINE, "opensearch")',
                    'opensearch_server_hostname' => '#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME, "127.0.0.1")',
                    'opensearch_server_port' => '#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT, "9200")',
                    'opensearch_index_prefix' => '#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_INDEX_PREFIX, "magento2")',
                    'opensearch_enable_auth' => '#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH, "0")',
                    'opensearch_server_timeout' => '#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_TIMEOUT, "15")',
                ],
            ],
            'smile_elasticsuite_core_base_settings' => [
                'es_client' => [
                    'servers' => '#env(MAGENTO_DC_ELASTICSUITE__ES_CLIENT__SERVERS, "127.0.0.1:9200")',
                    'enable_https_mode' => '#env(MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTPS_MODE, "0")',
                    'enable_http_auth' => '#env(MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTP_AUTH, "0")',
                ],
            ],
        ],
    ],
    'queue' => [
        'default_connection' => '#env(MAGENTO_DC_QUEUE__DEFAULT_CONNECTION, "db")',
        'amqp' => [
            'host' => '#env(MAGENTO_DC_QUEUE__AMQP__HOST, "127.0.0.1")',
            'port' => '#env(MAGENTO_DC_QUEUE__AMQP__PORT, "5672")',
            'ssl' => '#env(MAGENTO_DC_QUEUE__AMQP__SSL, "0")',
            // Magento's AMQP config contract calls this field "user". Keep the
            // environment variable named "USERNAME" for clarity at the
            // platform boundary, but emit the key Magento actually reads.
            'user' => '#env(MAGENTO_DC_QUEUE__AMQP__USERNAME, "magento")',
            'password' => '#env(MAGENTO_DC_QUEUE__AMQP__PASSWORD)',
        ],
        'stomp' => [
            'host' => '#env(MAGENTO_DC_QUEUE__STOMP__HOST, "127.0.0.1")',
            'port' => '#env(MAGENTO_DC_QUEUE__STOMP__PORT, "61613")',
            'ssl' => '#env(MAGENTO_DC_QUEUE__STOMP__SSL, "0")',
            'user' => '#env(MAGENTO_DC_QUEUE__STOMP__USER, "magento")',
            'password' => '#env(MAGENTO_DC_QUEUE__STOMP__PASSWORD)',
        ],
    ],
    'directories' => [
        'document_root_is_pub' => true,
    ],
    // Remote media storage (Magento RemoteStorage + AwsS3 driver). The
    // driver activates only when MAGENTO_DC_MEDIA__DRIVER names it; the
    // 'file' default keeps local media for runtimes without object
    // storage configured. GCP sets aws-s3 with GCS S3-interop endpoint
    // and HMAC credentials; every value defaults so an unset variable
    // can never break a runtime that does not use remote storage, while
    // a selected driver with empty bucket or credentials still fails
    // closed in the driver factory. Markers use MAGENTO_DC_* names:
    // the application image build rejects anything else here.
    'remote_storage' => [
        'driver' => '#env(MAGENTO_DC_MEDIA__DRIVER, "file")',
        'prefix' => '#env(MAGENTO_DC_MEDIA__PREFIX, "media/")',
        'config' => [
            'bucket' => '#env(MAGENTO_DC_MEDIA__BUCKET, "")',
            'region' => '#env(MAGENTO_DC_MEDIA__REGION, "")',
            'endpoint' => '#env(MAGENTO_DC_MEDIA__ENDPOINT, "https://storage.googleapis.com")',
            'credentials' => [
                'key' => '#env(MAGENTO_DC_MEDIA__KEY, "")',
                'secret' => '#env(MAGENTO_DC_MEDIA__SECRET, "")',
            ],
        ],
    ],
];
