<?php

declare(strict_types=1);

use MageLift\Project;

return new Project([
    'application' => [
        'edition' => 'open-source',
        'magento' => [
            'frontName' => 'admin',
        ],
        'mode' => 'integrated',
        'version' => '2.4.9',
        'webRuntime' => 'nginx-fpm',
    ],
    'build' => [
        'composer' => [
            'version' => '2.10+',
        ],
        'php' => '8.5',
    ],
    'defaults' => [
        'preset' => 'preview',
        'region' => 'eu-west-3',
    ],
    'environments' => [
        'preview' => [
            'account' => '123456789012',
            'class' => 'preview',
            'domain' => 'preview.example.test',
            'expiresAt' => '2027-12-31T23:59:59Z',
            'monthlyBudgetCents' => 5000,
            'preset' => 'preview',
        ],
    ],
    'extensions' => (object) [],
    'local' => [
        'database' => [
            'family' => 'mariadb',
            'version' => '11.8',
        ],
        'email' => [
            'mode' => 'mailpit',
        ],
    ],
    'project' => [
        'name' => 'synthetic-shop-aws',
    ],
    'schemaVersion' => 1,
    'target' => [
        'aws' => [
            'albCertificateArn' => 'arn:aws:acm:eu-west-3:123456789012:certificate/00000000-0000-0000-0000-000000000000',
            'availabilityZones' => [
                'eu-west-3a',
                'eu-west-3b',
            ],
            'cacheSecretArn' => 'arn:aws:secretsmanager:eu-west-3:123456789012:secret:example-invalid-cache-000000',
            'catalog' => [
                'databaseEngine' => 'rds-mysql',
                'queueMode' => 'db',
                'searchMode' => 'disabled',
                'version' => 'v1',
            ],
            'cloudFrontCertificateArn' => 'arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000',
            'databaseName' => 'magento',
            'encryptionKeySecretArn' => 'arn:aws:secretsmanager:eu-west-3:123456789012:secret:example-invalid-crypt-000000',
            'hostedZoneId' => 'Z000000000000000000000',
            'imageDigest' => 'registry.example.invalid/synthetic/app@sha256:0000000000000000000000000000000000000000000000000000000000000000',
            'kmsKeyArn' => 'arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000',
            'masterUsername' => 'magento',
            'mediaDomain' => 'media.example.test',
            'vpcCidr' => '10.42.0.0/16',
        ],
        'provider' => 'aws',
        'runtime' => 'ecs-fargate',
    ],
]);
