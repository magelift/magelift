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
        'region' => 'europe-west1',
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
        'name' => 'ci-gcp-fixture',
    ],
    'schemaVersion' => 1,
    'target' => [
        'gcp' => [
            'project' => 'example-gcp-project',
            'region' => 'europe-west1',
        ],
        'provider' => 'gcp',
        'runtime' => 'gke-autopilot',
    ],
]);
