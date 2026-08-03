<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Artifact;

use InvalidArgumentException;
use MageLift\Build\Artifact\ArtifactManifest;
use MageLift\Build\Artifact\CompatibilityStatus;
use MageLift\Build\Artifact\MagentoEdition;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class ArtifactManifestTest extends TestCase
{
    public function testSerializesCanonicalJsonRegardlessOfMapInsertionOrder(): void
    {
        $first = self::manifest(
            buildInputs: ['composer.lock' => 'first', 'app/etc/config.php' => 'second'],
            checksums: ['vendor/autoload.php' => str_repeat('b', 64), 'app/etc/config.php' => str_repeat('a', 64)],
        );
        $second = self::manifest(
            buildInputs: ['app/etc/config.php' => 'second', 'composer.lock' => 'first'],
            checksums: ['app/etc/config.php' => str_repeat('a', 64), 'vendor/autoload.php' => str_repeat('b', 64)],
            phpExtensions: ['pdo_mysql', 'intl'],
            requiredRuntimeCapabilities: ['cache.valkey', 'database.mysql'],
        );

        self::assertSame($first->toCanonicalJson(), $second->toCanonicalJson());
        self::assertSame(
            '{"buildInputs":{"app/etc/config.php":"second","composer.lock":"first"},"buildPackageVersion":"1.2.3","checksums":{"app/etc/config.php":"'.str_repeat('a', 64).'","vendor/autoload.php":"'.str_repeat('b', 64).'"},"compatibilityStatus":"supported","enabledModules":["Magento_Catalog"],"imageDigest":"sha256:'.str_repeat('d', 64).'","mageLiftVersion":"1.2.3","magento":{"edition":"open-source","version":"2.4.8"},"php":{"extensions":["intl","pdo_mysql"],"version":"8.4.1"},"requiredRuntimeCapabilities":["cache.valkey","database.mysql"],"sourceRevision":"'.str_repeat('c', 40).'","staticContentMatrix":{"en_US":["Magento/luma"]}}',
            $first->toCanonicalJson(),
        );
    }

    #[DataProvider('invalidManifests')]
    public function testRejectsInvalidPromotionMetadata(string $field, mixed $value, string $message): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage($message);

        self::manifest(...[$field => $value]);
    }

    public static function invalidManifests(): iterable
    {
        yield 'mutable revision' => ['sourceRevision', 'main', 'Source revision'];
        yield 'tag instead of digest' => ['imageDigest', 'ghcr.io/magelift/magelift:latest', 'Image digest'];
        yield 'partial PHP version' => ['phpVersion', '8.4', 'PHP version'];
        yield 'duplicate extensions' => ['phpExtensions', ['intl', 'intl'], 'cannot contain duplicates'];
        yield 'invalid module' => ['enabledModules', ['magento/catalog'], 'Invalid Magento module'];
        yield 'missing checksums' => ['checksums', [], 'checksums cannot be empty'];
        yield 'bad checksum' => ['checksums', ['file' => 'sha256:bad'], 'Invalid SHA-256 checksum'];
        yield 'unsafe checksum path' => ['checksums', ['../env.php' => str_repeat('a', 64)], 'cannot contain parent traversal'];
        yield 'missing themes' => ['staticContentMatrix', ['en_US' => []], 'non-empty theme list'];
        yield 'missing capabilities' => ['requiredRuntimeCapabilities', [], 'capabilities cannot be empty'];
        yield 'unstable capability ID' => ['requiredRuntimeCapabilities', ['Database MySQL'], 'Invalid runtime capability'];
    }

    /**
     * @param list<string> $phpExtensions
     * @param list<string> $enabledModules
     * @param array<string, string> $buildInputs
     * @param array<string, string> $checksums
     * @param array<string, list<string>> $staticContentMatrix
     * @param list<string> $requiredRuntimeCapabilities
     */
    private static function manifest(
        string $sourceRevision = 'cccccccccccccccccccccccccccccccccccccccc',
        string $imageDigest = 'sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd',
        string $mageLiftVersion = '1.2.3',
        string $buildPackageVersion = '1.2.3',
        MagentoEdition $magentoEdition = MagentoEdition::OpenSource,
        string $magentoVersion = '2.4.8',
        string $phpVersion = '8.4.1',
        array $phpExtensions = ['intl', 'pdo_mysql'],
        array $enabledModules = ['Magento_Catalog'],
        array $buildInputs = ['composer.lock' => 'first'],
        array $checksums = ['app/etc/config.php' => 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'],
        array $staticContentMatrix = ['en_US' => ['Magento/luma']],
        array $requiredRuntimeCapabilities = ['database.mysql', 'cache.valkey'],
        CompatibilityStatus $compatibilityStatus = CompatibilityStatus::Supported,
    ): ArtifactManifest {
        return new ArtifactManifest(
            $sourceRevision,
            $imageDigest,
            $mageLiftVersion,
            $buildPackageVersion,
            $magentoEdition,
            $magentoVersion,
            $phpVersion,
            $phpExtensions,
            $enabledModules,
            $buildInputs,
            $checksums,
            $staticContentMatrix,
            $requiredRuntimeCapabilities,
            $compatibilityStatus,
        );
    }
}
