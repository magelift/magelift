<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Artifact;

use MageLift\Build\Artifact\ArtifactManifest;
use MageLift\Build\Artifact\ArtifactManifestWriter;
use MageLift\Build\Artifact\CompatibilityStatus;
use MageLift\Build\Artifact\MagentoEdition;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class ArtifactManifestWriterTest extends TestCase
{
    public function testWritesCanonicalManifestAtomically(): void
    {
        $directory = sys_get_temp_dir().'/magelift-manifest-'.bin2hex(random_bytes(8));
        self::assertTrue(mkdir($directory, 0700));
        $path = $directory.'/manifest.json';

        try {
            (new ArtifactManifestWriter())->write(self::manifest(), $path);

            self::assertSame(self::manifest()->toCanonicalJson()."\n", file_get_contents($path));
            self::assertSame(0644, fileperms($path) & 0777);
            self::assertSame([], glob($directory.'/.magelift-manifest-*'));
        } finally {
            @unlink($path);
            @rmdir($directory);
        }
    }

    public function testRejectsMissingDestinationDirectory(): void
    {
        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('does not exist');

        (new ArtifactManifestWriter())->write(self::manifest(), sys_get_temp_dir().'/missing-'.bin2hex(random_bytes(8)).'/manifest.json');
    }

    private static function manifest(): ArtifactManifest
    {
        return new ArtifactManifest(
            str_repeat('c', 40),
            'sha256:'.str_repeat('d', 64),
            '1.2.3',
            '1.2.3',
            MagentoEdition::OpenSource,
            '2.4.8',
            '8.4.1',
            ['intl', 'pdo_mysql'],
            ['Magento_Catalog'],
            ['composer.lock' => 'locked'],
            ['composer.lock' => str_repeat('a', 64)],
            ['en_US' => ['Magento/luma']],
            ['database.mysql'],
            CompatibilityStatus::Supported,
        );
    }
}
