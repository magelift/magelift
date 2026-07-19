<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Protocol;

use InvalidArgumentException;
use MageLift\Build\Protocol\FinalizeResponse;
use MageLift\Build\Protocol\PrepareResponse;
use MageLift\Build\Protocol\ProtocolException;
use PHPUnit\Framework\TestCase;

final class ResponseTest extends TestCase
{
    public function testPrepareGoldenFixtureMatchesGoCodecByteForByte(): void
    {
        $response = new PrepareResponse(
            'dist/rootfs.tar',
            '8.5.1',
            ['pdo_mysql', 'intl'],
            ['Vendor_Second', 'Magento_Catalog'],
            [
                ['path' => 'vendor/autoload.php', 'sha256' => str_repeat('d', 64)],
                ['path' => 'app/etc/config.php', 'sha256' => str_repeat('e', 64)],
            ],
            ['search.opensearch', 'database.mysql'],
        );

        self::assertSame(
            '{"protocolVersion":1,"stage":"prepare","prepare":{"preparedArtifact":"dist/rootfs.tar","phpVersion":"8.5.1","phpExtensions":["intl","pdo_mysql"],"enabledModules":["Magento_Catalog","Vendor_Second"],"checksums":[{"path":"app/etc/config.php","sha256":"'.str_repeat('e', 64).'"},{"path":"vendor/autoload.php","sha256":"'.str_repeat('d', 64).'"}],"requiredRuntimeCapabilities":["database.mysql","search.opensearch"]}}',
            $response->toCanonicalJson(),
        );
    }

    public function testFinalizeGoldenFixtureMatchesGoCodecByteForByte(): void
    {
        $response = new FinalizeResponse(
            'sha256:'.str_repeat('b', 64),
            'dist/manifest.json',
            str_repeat('c', 64),
        );

        self::assertSame(
            '{"protocolVersion":1,"stage":"finalize","finalize":{"imageDigest":"sha256:'.str_repeat('b', 64).'","manifestPath":"dist/manifest.json","manifestSha256":"'.str_repeat('c', 64).'"}}',
            $response->toCanonicalJson(),
        );
    }

    public function testRejectsDuplicateResponseSetValues(): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage('Duplicate PHP extensions');

        new PrepareResponse(
            'dist/rootfs.tar',
            '8.5.1',
            ['intl', 'intl'],
            ['Magento_Catalog'],
            [['path' => 'file', 'sha256' => str_repeat('a', 64)]],
            ['database.mysql'],
        );
    }

    public function testRejectsOversizedResponse(): void
    {
        $response = new PrepareResponse(
            'dist/'.str_repeat('x', 1_048_576),
            '8.5.1',
            ['intl'],
            ['Magento_Catalog'],
            [['path' => 'file', 'sha256' => str_repeat('a', 64)]],
            ['database.mysql'],
        );

        $this->expectException(ProtocolException::class);
        $this->expectExceptionMessage('1048576-byte limit');
        $response->toCanonicalJson();
    }
}
