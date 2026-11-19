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
            'credentials' => 'aws-secrets-manager://magelift/composer',
            'version' => '2.10',
        ],
        'extensions' => [
            'apcu',
            'bcmath',
            'ftp',
            'gd',
            'intl',
            'mbstring',
            'opcache',
            'pdo_mysql',
            'redis',
            'soap',
            'sockets',
            'sodium',
            'xsl',
            'zip',
        ],
        'php' => '8.5',
        'staticContent' => [
            'locales' => [
                'en_US',
            ],
            'themes' => [
                'Magento/luma',
            ],
        ],
    ],
    'defaults' => [
        'preset' => 'preview',
        'region' => 'eu-west-3',
    ],
    'environments' => [
        'preview' => [
            'account' => '123456789012',
            'class' => 'preview',
            'domain' => 'preview.example.com',
            'expiresAt' => '2026-12-31T23:59:59Z',
            'monthlyBudgetCents' => 5000,
            'preset' => 'preview',
        ],
        'production' => [
            'account' => '210987654321',
            'class' => 'production',
            'domain' => 'example.com',
            'inherits' => 'staging',
            'preset' => 'high-availability',
            'protection' => true,
        ],
        'staging' => [
            'account' => '123456789012',
            'class' => 'staging',
            'domain' => 'staging.example.com',
            'inherits' => 'preview',
            'preset' => 'standard',
            'target' => [
                'aws' => [
                    'catalog' => [
                        'queueMode' => 'ecs-rabbitmq',
                    ],
                ],
            ],
        ],
    ],
    'extensions' => (object) [],
    'local' => [
        'email' => [
            'mode' => 'mailpit',
        ],
    ],
    'project' => [
        'name' => 'sample-shop',
    ],
    'schemaVersion' => 1,
    'target' => [
        'aws' => [
            'catalog' => [
                'queueMode' => 'db',
            ],
        ],
        'provider' => 'aws',
        'runtime' => 'ecs-fargate',
    ],
]);
