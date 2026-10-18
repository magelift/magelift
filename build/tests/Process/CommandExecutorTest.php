<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Process;

use InvalidArgumentException;
use MageLift\Build\Magento\Command;
use MageLift\Build\Magento\Executable;
use MageLift\Build\Process\CommandExecutor;
use MageLift\Build\Process\ProcessFailed;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;
use PHPUnit\Framework\TestCase;

final class CommandExecutorTest extends TestCase
{
    public function testPassesArgvAndAllowedContextToRunner(): void
    {
        $runner = new RecordingRunner(new ProcessResult(0, 'done', ''));
        $directory = sys_get_temp_dir();
        $executor = new CommandExecutor($runner, ['MAGENTO_MODE'], [$directory]);

        $result = $executor->execute(
            new Command(Executable::Magento, ['cache:flush', 'argument with spaces']),
            $directory,
            ['MAGENTO_MODE' => 'developer'],
            12.5,
        );

        self::assertTrue($result->succeeded());
        self::assertSame(
            ['bin/magento', 'cache:flush', 'argument with spaces'],
            $runner->request?->argv,
        );
        self::assertSame(['MAGENTO_MODE' => 'developer'], $runner->request?->environment);
        self::assertSame(12.5, $runner->request?->timeoutSeconds);
    }

    public function testRejectsEnvironmentOutsideAllowlist(): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage('"SECRET" is not allowed');

        (new CommandExecutor(new RecordingRunner(new ProcessResult(0, '', ''))))->execute(
            new Command(Executable::Composer, ['validate']),
            getcwd(),
            ['SECRET' => 'plaintext'],
        );
    }

    public function testRejectsWorkingDirectoryOutsideAllowlist(): void
    {
        $allowed = sys_get_temp_dir().DIRECTORY_SEPARATOR.'magelift-allowed-'.bin2hex(random_bytes(4));
        mkdir($allowed);

        try {
            $this->expectException(InvalidArgumentException::class);
            $this->expectExceptionMessage('is not allowed');
            (new CommandExecutor(
                new RecordingRunner(new ProcessResult(0, '', '')),
                workingDirectoryAllowlist: [$allowed],
            ))->execute(new Command(Executable::Composer, ['validate']), getcwd());
        } finally {
            rmdir($allowed);
        }
    }

    public function testExecuteOrFailExposesStableResult(): void
    {
        $result = new ProcessResult(7, 'partial output', 'failure details');
        $executor = new CommandExecutor(new RecordingRunner($result));

        try {
            $executor->executeOrFail(new Command(Executable::Composer, ['validate']), getcwd());
            self::fail('Expected process failure.');
        } catch (ProcessFailed $failure) {
            self::assertSame($result, $failure->result);
            self::assertSame('Process exited with code 7.', $failure->getMessage());
        }
    }
}

final class RecordingRunner implements ProcessRunner
{
    public ?ProcessRequest $request = null;

    public function __construct(private readonly ProcessResult $result)
    {
    }

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->request = $request;

        return $this->result;
    }
}
