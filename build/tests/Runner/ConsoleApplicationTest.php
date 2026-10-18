<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Runner;

use MageLift\Build\Runner\ConsoleApplication;
use PHPUnit\Framework\TestCase;

final class ConsoleApplicationTest extends TestCase
{
    public function testWritesOnlyCanonicalResponseToStdout(): void
    {
        [$service] = RunnerServiceTestAccess::service();
        [$stdin, $stdout, $stderr] = self::streams(RunnerServiceTestAccess::prepareRequest());

        $exit = (new ConsoleApplication($service))->run($stdin, $stdout, $stderr);

        self::assertSame(ConsoleApplication::EXIT_SUCCESS, $exit);
        self::assertSame('', self::contents($stderr));
        self::assertStringStartsWith('{"protocolVersion":1,"stage":"prepare"', self::contents($stdout));
    }

    public function testReturnsStableInvalidRequestExitAndKeepsDiagnosticsOffStdout(): void
    {
        [$service] = RunnerServiceTestAccess::service();
        [$stdin, $stdout, $stderr] = self::streams('{');

        $exit = (new ConsoleApplication($service))->run($stdin, $stdout, $stderr);

        self::assertSame(ConsoleApplication::EXIT_INVALID_REQUEST, $exit);
        self::assertSame('', self::contents($stdout));
        self::assertStringStartsWith('invalid request:', self::contents($stderr));
    }

    /** @return array{resource, resource, resource} */
    private static function streams(string $input): array
    {
        $streams = [fopen('php://temp', 'w+'), fopen('php://temp', 'w+'), fopen('php://temp', 'w+')];
        fwrite($streams[0], $input);
        rewind($streams[0]);

        return $streams;
    }

    /** @param resource $stream */
    private static function contents($stream): string
    {
        rewind($stream);
        return (string) stream_get_contents($stream);
    }
}

final class RunnerServiceTestAccess
{
    public static function service(): array
    {
        $filesystem = new MemoryFilesystem(['/repo/composer.lock' => str_repeat('b', 64)]);
        return [new \MageLift\Build\Runner\RunnerService(
            $filesystem,
            new MatchingRevision(),
            new FakePreparation(),
            '/runner-state',
            '1.2.3',
            '1.2.3',
        )];
    }

    public static function prepareRequest(): string
    {
        return '{"protocolVersion":1,"stage":"prepare","prepare":{"repositoryRoot":"/repo","sourceRevision":"'.str_repeat('a', 40).'","application":{"edition":"open-source","version":"2.4.9","mode":"integrated","webRuntime":"nginx-fpm"},"phpVersion":"8.5","compatibilityStatus":"unsupported-allowed","inputFiles":[{"path":"composer.lock","sha256":"'.str_repeat('b', 64).'"}],"staticContent":[{"locale":"en_US","theme":"Magento/luma"}]}}';
    }
}
