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
        'staging' => [
            'account' => '123456789012',
            'domain' => 'staging.example.invalid',
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
        'name' => 'fixture-shop',
    ],
    'schemaVersion' => 1,
    'target' => [
        'aws' => (object) [],
        'provider' => 'aws',
        'runtime' => 'ecs-fargate',
    ],
]);
