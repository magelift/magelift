<?php

declare(strict_types=1);

use MageLift\Project;

return new Project([
    'application' => [
        'edition' => 'open-source',
        'mode' => 'integrated',
        'version' => '2.4.9',
    ],
    'build' => [
        'composer' => [
            'credentials' => 'aws-secrets-manager://magelift/example/composer',
            'version' => '2.10',
        ],
        'extensions' => [
            'apcu',
            'bcmath',
            'gd',
            'intl',
            'mbstring',
            'opcache',
            'pdo_mysql',
            'redis',
            'sodium',
            'zip',
        ],
        'php' => '8.5',
    ],
    'defaults' => [
        'preset' => 'standard',
        'region' => 'eu-west-3',
    ],
    'edge' => [
        'dnsMode' => 'external',
        'domains' => [
            'shop.example.com',
        ],
        'externalProvider' => 'fastly',
        'health' => [
            'expectedCname' => 'dualstack.e.sni.global.fastly.net',
            'expectedStatus' => 200,
            'originHost' => 'origin.example.com',
            'originUrl' => 'https://origin.example.com/health',
            'routePath' => '/health',
            'routePollSeconds' => 5,
            'routeTimeoutSeconds' => 300,
        ],
        'mode' => 'both',
        'nativeProvider' => 'cloudfront',
        'originHealthRef' => 'example/origin-health',
        'serviceId' => 'replace-with-fastly-service-id',
        'tls' => true,
        'tlsMode' => 'external',
        'tokenSecret' => 'aws-secrets-manager://magelift/example/fastly-token',
    ],
    'environments' => [
        'staging' => [
            'account' => '123456789012',
            'class' => 'staging',
            'domain' => 'shop.example.com',
        ],
    ],
    'extensions' => [
        'newrelic.otlp' => [
            'collectorMode' => 'direct-otlp',
            'signalSet' => [
                'logs',
                'metrics',
                'traces',
            ],
        ],
    ],
    'observability' => [
        'credentialReferences' => [
            'aws-secrets-manager://magelift/example/newrelic-license',
        ],
        'endpoint' => 'https://otlp.eu01.nr-data.net',
        'environment' => 'staging',
        'externalProvider' => 'newrelic',
        'labels' => [
            'cost-center' => 'commerce',
            'owner' => 'platform',
        ],
        'logs' => true,
        'metrics' => true,
        'nativeProvider' => 'cloudwatch',
        'redactionPolicyRef' => 'example/default-redaction',
        'retentionDays' => 30,
        'samplingRatio' => 0.2,
        'serviceName' => 'example-shop',
        'traces' => true,
    ],
    'project' => [
        'name' => 'example-shop',
    ],
    'resilience' => [
        'availabilityTarget' => 'multi-az',
        'failoverOwner' => 'platform-oncall',
        'fencingPolicy' => 'single-writer-lease',
        'profileId' => 'standard-multi-az',
        'projection' => [
            'cluster' => 'example-shop',
            'container' => 'php',
            'runtime' => 'ecs',
            'service' => 'web',
        ],
        'recoveryDestinations' => [
            'same-region',
            'same-region-isolated',
        ],
        'recoveryScope' => 'same-region-and-isolated-destination',
        'retentionDays' => 14,
        'rpoSeconds' => 900,
        'rtoSeconds' => 1800,
    ],
    'schemaVersion' => 1,
    'target' => [
        'aws' => [
            'availabilityZones' => [
                'eu-west-3a',
                'eu-west-3b',
            ],
            'catalog' => [
                'cacheSnapshotRetentionLimit' => 7,
                'cacheSnapshotWindow' => '04:00-05:00',
                'databaseBackupWindow' => '03:00-04:00',
                'databaseDeleteAutomatedBackups' => false,
                'databaseDeletionProtection' => true,
                'databaseMaintenanceWindow' => 'sun:05:00-sun:06:00',
                'queueMode' => 'db',
                'searchMode' => 'disabled',
            ],
            'natInstanceType' => 'c6gn.medium',
            'natMode' => 'fck-nat',
            'natReplacementMode' => 'auto-scaling',
            'natTopology' => 'multi-az',
            'vpcCidr' => '10.42.0.0/16',
        ],
        'provider' => 'aws',
        'runtime' => 'ecs-fargate',
    ],
]);
