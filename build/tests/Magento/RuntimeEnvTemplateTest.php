<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use PHPUnit\Framework\TestCase;

// RuntimeEnvTemplateTest pins the baked env.php contract: every value the
// platform promises resolves from the documented environment variable, and
// remote media storage stays inert unless a driver is selected.
final class RuntimeEnvTemplateTest extends TestCase
{
    public function testRemoteStorageSectionUsesDocumentedVariables(): void
    {
        $env = require dirname(__DIR__, 3) . '/images/php-nginx/env.php';

        self::assertSame('#env(MAGELIFT_MEDIA_DRIVER, "file")', $env['remote_storage']['driver']);
        self::assertSame('#env(MAGELIFT_MEDIA_S3_PREFIX, "media/")', $env['remote_storage']['prefix']);
        self::assertSame('#env(MAGELIFT_MEDIA_BUCKET, "")', $env['remote_storage']['config']['bucket']);
        self::assertSame('#env(MAGELIFT_MEDIA_S3_REGION, "")', $env['remote_storage']['config']['region']);
        self::assertSame(
            '#env(MAGELIFT_MEDIA_S3_ENDPOINT, "https://storage.googleapis.com")',
            $env['remote_storage']['config']['endpoint']
        );
        self::assertSame('#env(MAGELIFT_MEDIA_S3_KEY, "")', $env['remote_storage']['config']['credentials']['key']);
        self::assertSame('#env(MAGELIFT_MEDIA_S3_SECRET, "")', $env['remote_storage']['config']['credentials']['secret']);
    }
}
