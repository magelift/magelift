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
        'php' => '8.5',
    ],
    'defaults' => [
        'preset' => 'preview',
        'region' => 'westeurope',
    ],
    'environments' => [
        'preview' => [
            'class' => 'preview',
            'domain' => 'preview.example.com',
            'expiresAt' => '2030-01-01T00:00:00Z',
        ],
        'staging' => [
            'class' => 'staging',
            'domain' => 'staging.example.com',
        ],
    ],
    'extensions' => (object) [],
    'project' => [
        'name' => 'ci-azure-fixture',
    ],
    'schemaVersion' => 1,
    'target' => [
        'azure' => [
            'subscriptionId' => '00000000-0000-0000-0000-000000000001',
            'resourceGroup' => 'ci-azure-fixture',
        ],
        'provider' => 'azure',
        'runtime' => 'aks',
    ],
]);
