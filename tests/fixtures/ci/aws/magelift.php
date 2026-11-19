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
        'region' => 'eu-west-3',
    ],
    'environments' => [
        'preview' => [
            'account' => '123456789012',
            'class' => 'preview',
            'domain' => 'preview.example.com',
            'expiresAt' => '2030-01-01T00:00:00Z',
        ],
        'staging' => [
            'account' => '123456789012',
            'class' => 'staging',
            'domain' => 'staging.example.com',
        ],
    ],
    'extensions' => (object) [],
    'project' => [
        'name' => 'ci-aws-fixture',
    ],
    'schemaVersion' => 1,
    'target' => [
        'aws' => (object) [],
        'provider' => 'aws',
        'runtime' => 'ecs-fargate',
    ],
]);
