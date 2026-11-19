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
        'region' => 'europe-west1',
    ],
    'environments' => [
        'preview' => [
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
        'name' => 'synthetic-shop',
    ],
    'schemaVersion' => 1,
    'target' => [
        'gcp' => [
            'encryptionKeySecret' => 'example-invalid-magento-crypt-key',
            'imageDigest' => 'registry.example.invalid/synthetic/app@sha256:0000000000000000000000000000000000000000000000000000000000000000',
            'openSearchMode' => 'disabled',
            'project' => 'example-invalid-preview',
            'queueMode' => 'database',
            'region' => 'europe-west1',
        ],
        'provider' => 'gcp',
        'runtime' => 'gke-autopilot',
    ],
]);
