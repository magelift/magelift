<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Runner;

use MageLift\Build\Protocol\PrepareRequest;
use MageLift\Build\Runner\Preparation;
use MageLift\Build\Runner\PreparationOutput;
use MageLift\Build\Runner\RevisionVerifier;
use MageLift\Build\Runner\RunnerFilesystem;
use MageLift\Build\Runner\RunnerService;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class RunnerServiceTest extends TestCase
{
    public function testPrepareValidatesInputsRunsLifecycleAndWritesPrivateExternalMetadata(): void
    {
        [$service, $filesystem, $preparation] = self::service();

        $response = $service->handle(self::prepareRequest());

        self::assertStringContainsString('"stage":"prepare"', $response);
        self::assertStringContainsString('"phpVersion":"8.5.4"', $response);
        self::assertSame(1, $preparation->calls);
        $metadataPath = '/runner-state/prepared/'.str_repeat('a', 40).'.json';
        self::assertArrayHasKey($metadataPath, $filesystem->writes);
        self::assertSame(0o600, $filesystem->writes[$metadataPath]['permissions']);
        self::assertStringNotContainsString('sha256:', $filesystem->writes[$metadataPath]['contents']);
        self::assertStringStartsNotWith('/repo/', $metadataPath);
    }

    public function testPrepareStopsBeforeLifecycleOnChecksumMismatch(): void
    {
        [$service, $filesystem, $preparation] = self::service();
        $filesystem->checksums['/repo/composer.lock'] = str_repeat('f', 64);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('checksum mismatch');
        try {
            $service->handle(self::prepareRequest());
        } finally {
            self::assertSame(0, $preparation->calls);
        }
    }

    public function testFinalizeWritesCanonicalExternalManifestAtomicallyAndReturnsChecksum(): void
    {
        [$service, $filesystem] = self::service();
        $service->handle(self::prepareRequest());
        $digest = 'sha256:'.str_repeat('d', 64);

        $response = $service->handle(self::finalizeRequest($digest));

        $manifestPath = '/runner-state/manifests/'.str_repeat('a', 40).'.json';
        $manifest = $filesystem->writes[$manifestPath]['contents'];
        self::assertSame(0o644, $filesystem->writes[$manifestPath]['permissions']);
        self::assertStringContainsString('"imageDigest":"'.$digest.'"', $manifest);
        self::assertStringContainsString('"compatibilityStatus":"unsupported-allowed"', $manifest);
        self::assertStringContainsString('"manifestSha256":"'.hash('sha256', $manifest).'"', $response);
        self::assertStringContainsString('"manifestPath":"manifests/'.str_repeat('a', 40).'.json"', $response);
    }

    public function testFinalizeRejectsRevisionDifferentFromPreparedMetadata(): void
    {
        [$service] = self::service();
        $service->handle(self::prepareRequest());

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('does not match prepared metadata');
        $service->handle(self::finalizeRequest('sha256:'.str_repeat('d', 64), str_repeat('e', 40)));
    }

    /** @return array{RunnerService, MemoryFilesystem, FakePreparation} */
    private static function service(): array
    {
        $filesystem = new MemoryFilesystem([
            '/repo/composer.lock' => str_repeat('b', 64),
        ]);
        $preparation = new FakePreparation();
        $service = new RunnerService(
            $filesystem,
            new MatchingRevision(),
            $preparation,
            '/runner-state',
            '1.2.3',
            '1.2.3',
        );

        return [$service, $filesystem, $preparation];
    }

    private static function prepareRequest(): string
    {
        return '{"protocolVersion":1,"stage":"prepare","prepare":{"repositoryRoot":"/repo","sourceRevision":"'.str_repeat('a', 40).'","application":{"edition":"open-source","version":"2.4.9","mode":"integrated","webRuntime":"nginx-fpm"},"phpVersion":"8.5","compatibilityStatus":"unsupported-allowed","inputFiles":[{"path":"composer.lock","sha256":"'.str_repeat('b', 64).'"}],"staticContent":[{"locale":"en_US","theme":"Magento/luma"}]}}';
    }

    private static function finalizeRequest(string $digest, string $revision = ''): string
    {
        $revision = $revision ?: str_repeat('a', 40);
        return '{"protocolVersion":1,"stage":"finalize","finalize":{"preparedArtifact":"prepared/'.str_repeat('a', 40).'.json","sourceRevision":"'.$revision.'","imageDigest":"'.$digest.'"}}';
    }
}

final class MemoryFilesystem implements RunnerFilesystem
{
    /** @var array<string, array{contents: string, permissions: int}> */
    public array $writes = [];

    /** @param array<string, string> $checksums */
    public function __construct(public array $checksums)
    {
    }

    public function checksum(string $path): string
    {
        return isset($this->writes[$path])
            ? hash('sha256', $this->writes[$path]['contents'])
            : ($this->checksums[$path] ?? throw new RuntimeException('missing file'));
    }

    public function checksumWithin(string $root, string $relativePath): string
    {
        return $this->checksum(rtrim($root, '/').'/'.$relativePath);
    }

    public function read(string $path): string
    {
        return $this->writes[$path]['contents'] ?? throw new RuntimeException('missing file');
    }

    public function atomicWrite(string $path, string $contents, int $permissions): void
    {
        $this->writes[$path] = ['contents' => $contents, 'permissions' => $permissions];
    }
}

final class MatchingRevision implements RevisionVerifier
{
    public function matches(string $repositoryRoot, string $sourceRevision): bool
    {
        return $repositoryRoot === '/repo' && $sourceRevision === str_repeat('a', 40);
    }
}

final class FakePreparation implements Preparation
{
    public int $calls = 0;

    public function prepare(PrepareRequest $request): PreparationOutput
    {
        ++$this->calls;

        return new PreparationOutput(
            '8.5.4',
            ['intl', 'pdo_mysql'],
            ['Magento_Catalog'],
            [['path' => 'app/etc/config.php', 'sha256' => str_repeat('c', 64)]],
            ['cache.valkey', 'database.mysql'],
        );
    }
}
