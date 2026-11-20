<?php

declare(strict_types=1);

use MageLift\Project;

return new Project([
    'schemaVersion' => 1,
    'project' => ['name' => 'build-fixture'],
    'target' => [
        'provider' => 'aws',
        'runtime' => 'ecs-fargate',
        'aws' => (object) [],
    ],
    'defaults' => ['preset' => 'preview', 'region' => 'eu-west-3'],
    'application' => [
        'edition' => 'open-source',
        'version' => '2.4.9',
        'mode' => 'integrated',
    ],
    'build' => [
        'php' => '8.5',
        'staticContent' => ['matrix' => ['Fixture/store' => []]],
    ],
    'environments' => [
        'staging' => [
            'class' => 'staging',
            'account' => '123456789012',
            'domain' => 'shop.example.test',
        ],
    ],
]);
