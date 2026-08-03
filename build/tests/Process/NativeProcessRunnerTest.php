<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Process;

use MageLift\Build\Process\NativeProcessRunner;
use MageLift\Build\Process\ProcessRequest;
use PHPUnit\Framework\TestCase;

final class NativeProcessRunnerTest extends TestCase
{
    public function testExecutesArgumentVectorWithoutShellInterpolationAndCapturesOutput(): void
    {
        $argument = '$(touch should-not-exist); echo unsafe';
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', 'fwrite(STDOUT, $argv[1]); fwrite(STDERR, "diagnostic");', $argument],
            getcwd(),
        ));

        self::assertTrue($result->succeeded());
        self::assertSame($argument, $result->stdout);
        self::assertSame('diagnostic', $result->stderr);
        self::assertFileDoesNotExist(getcwd().DIRECTORY_SEPARATOR.'should-not-exist');
    }

    public function testReturnsNonZeroExitWithoutLosingOutput(): void
    {
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', 'fwrite(STDOUT, "before"); fwrite(STDERR, "failed"); exit(23);'],
            getcwd(),
        ));

        self::assertFalse($result->succeeded());
        self::assertSame(23, $result->exitCode);
        self::assertSame('before', $result->stdout);
        self::assertSame('failed', $result->stderr);
    }

    public function testTerminatesTimedOutProcess(): void
    {
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', 'usleep(500000);'],
            getcwd(),
            timeoutSeconds: 0.02,
        ));

        self::assertFalse($result->succeeded());
        self::assertTrue($result->timedOut);
        self::assertFalse($result->cancelled);
    }

    public function testTerminatesCancelledProcess(): void
    {
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', 'usleep(500000);'],
            getcwd(),
            timeoutSeconds: 1,
            isCancelled: static fn (): bool => true,
        ));

        self::assertFalse($result->succeeded());
        self::assertTrue($result->cancelled);
        self::assertFalse($result->timedOut);
    }
}
