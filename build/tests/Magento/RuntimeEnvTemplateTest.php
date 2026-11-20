<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use PHPUnit\Framework\TestCase;

final class RuntimeEnvTemplateTest extends TestCase
{
    public function testMagentoSettingsMatchTheRuntimeInputs(): void
    {
        $recipe = dirname(__DIR__, 3) . '/images/php-nginx';
        $env = require $recipe . '/env.php';
        $amqp = $env['queue']['amqp'];
        self::assertSame('#env(MAGENTO_DC_QUEUE__AMQP__USERNAME, "magento")', $amqp['user']);
        self::assertSame('#env(MAGENTO_DC_QUEUE__AMQP__PASSWORD)', $amqp['password']);
        self::assertArrayNotHasKey('username', $amqp);
        self::assertSame('#env(MAGENTO_DC_INSTALL__DATE, "Thu, 01 Jan 1970 00:00:00 +0000")', $env['install']['date']);
        self::assertArrayNotHasKey('port', $env['db']['connection']['default']);
        self::assertSame(
            '#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME, "127.0.0.1")',
            $env['system']['default']['catalog']['search']['opensearch_server_hostname']
        );
        self::assertArrayNotHasKey('catalog', $env);

        $pool = file_get_contents($recipe . '/www.conf');
        self::assertIsString($pool);
        self::assertMatchesRegularExpression('/^\s*clear_env\s*=\s*no\s*$/m', $pool);
    }

    public function testRemoteStorageSectionUsesDocumentedVariables(): void
    {
        $env = require dirname(__DIR__, 3) . '/images/php-nginx/env.php';

        self::assertSame('#env(MAGENTO_DC_MEDIA__DRIVER, "file")', $env['remote_storage']['driver']);
        self::assertSame('#env(MAGENTO_DC_MEDIA__PREFIX, "")', $env['remote_storage']['prefix']);
        self::assertSame('#env(MAGENTO_DC_MEDIA__BUCKET, "")', $env['remote_storage']['config']['bucket']);
        self::assertSame('#env(MAGENTO_DC_MEDIA__REGION, "")', $env['remote_storage']['config']['region']);
        self::assertSame(
            '#env(MAGENTO_DC_MEDIA__ENDPOINT, "https://storage.googleapis.com")',
            $env['remote_storage']['config']['endpoint']
        );
        self::assertSame('#env(MAGENTO_DC_MEDIA__KEY, "")', $env['remote_storage']['config']['credentials']['key']);
        self::assertSame('#env(MAGENTO_DC_MEDIA__SECRET, "")', $env['remote_storage']['config']['credentials']['secret']);
    }
}
