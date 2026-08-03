<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Lifecycle;

use MageLift\Build\Lifecycle\LifecycleExecutor;
use MageLift\Build\Lifecycle\LifecycleGraph;
use MageLift\Build\Lifecycle\FailureAction;
use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\RetryPolicy;
use MageLift\Build\Lifecycle\Step;
use MageLift\Build\Lifecycle\StepCommandProvider;
use MageLift\Build\Lifecycle\StepInterface;
use MageLift\Build\Magento\Command;
use MageLift\Build\Magento\CommandInterface;
use MageLift\Build\Magento\Executable;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class LifecycleExecutorTest extends TestCase
{
    public function testExecutesStepsInDependencyOrderAndReturnsStructuredResults(): void
    {
        $runner = new SequenceRunner([
            new ProcessResult(0, 'validated', ''),
            new ProcessResult(0, 'compiled', ''),
            new ProcessResult(0, 'packaged', ''),
        ]);
        $provider = new RecordingCommandProvider();
        $graph = new LifecycleGraph([
            new Step('package.image', Phase::Package, ['build.compile']),
            new Step('build.compile', Phase::Build, ['validate.config']),
            new Step('validate.config', Phase::Validate),
        ]);

        $result = (new LifecycleExecutor($runner, $provider))->execute($graph, getcwd());

        self::assertTrue($result->succeeded());
        self::assertNull($result->failedStepId);
        self::assertSame(
            ['validate.config', 'build.compile', 'package.image'],
            array_map(static fn ($step): string => $step->stepId, $result->steps),
        );
        self::assertSame(
            ['validate.config', 'build.compile', 'package.image'],
            $provider->requestedStepIds,
        );
    }

    public function testStopsAtFirstFailedStep(): void
    {
        $runner = new SequenceRunner([
            new ProcessResult(0, '', ''),
            new ProcessResult(9, '', 'compile failed'),
            new ProcessResult(0, '', ''),
        ]);
        $graph = new LifecycleGraph([
            new Step('validate.config', Phase::Validate),
            new Step('build.compile', Phase::Build, ['validate.config']),
            new Step('package.image', Phase::Package, ['build.compile']),
        ]);

        $result = (new LifecycleExecutor($runner, new RecordingCommandProvider()))->execute($graph, getcwd());

        self::assertFalse($result->succeeded());
        self::assertSame('build.compile', $result->failedStepId);
        self::assertSame(
            ['validate.config', 'build.compile'],
            array_map(static fn ($step): string => $step->stepId, $result->steps),
        );
        self::assertCount(2, $runner->requests);
        self::assertSame('compile failed', $result->steps[1]->attempts[0][0]->stderr);
    }

    public function testRetriesOnlyAccordingToStepPolicy(): void
    {
        $runner = new SequenceRunner([
            new ProcessResult(1, '', 'temporary failure'),
            new ProcessResult(0, 'ok', ''),
        ]);
        $delays = [];
        $executor = new LifecycleExecutor(
            $runner,
            new RecordingCommandProvider(),
            static function (int $seconds) use (&$delays): void {
                $delays[] = $seconds;
            },
        );
        $graph = new LifecycleGraph([
            new Step(
                'build.compile',
                Phase::Build,
                timeout: 17,
                retries: new RetryPolicy(2, 3, true),
            ),
        ]);

        $result = $executor->execute($graph, getcwd());

        self::assertTrue($result->succeeded());
        self::assertCount(2, $result->steps[0]->attempts);
        self::assertSame([3], $delays);
        self::assertSame(17.0, $runner->requests[0]->timeoutSeconds);
    }

    public function testStopsRemainingCommandsWithinFailedStep(): void
    {
        $runner = new SequenceRunner([
            new ProcessResult(4, '', 'failed'),
            new ProcessResult(0, '', ''),
        ]);
        $provider = new RecordingCommandProvider(commandsPerStep: 2);
        $graph = new LifecycleGraph([new Step('deploy.upgrade', Phase::Deploy)]);

        $result = (new LifecycleExecutor($runner, $provider))->execute($graph, getcwd());

        self::assertFalse($result->succeeded());
        self::assertCount(1, $runner->requests);
        self::assertCount(1, $result->steps[0]->attempts[0]);
    }

    public function testDoesNotRetryCancelledStep(): void
    {
        $runner = new SequenceRunner([
            new ProcessResult(1, '', '', cancelled: true),
            new ProcessResult(0, '', ''),
        ]);
        $delays = [];
        $executor = new LifecycleExecutor(
            $runner,
            new RecordingCommandProvider(),
            static function (int $seconds) use (&$delays): void {
                $delays[] = $seconds;
            },
        );
        $graph = new LifecycleGraph([
            new Step('build.compile', Phase::Build, retries: new RetryPolicy(2, 3, true)),
        ]);

        $result = $executor->execute($graph, getcwd());

        self::assertFalse($result->succeeded());
        self::assertCount(1, $runner->requests);
        self::assertSame([], $delays);
    }

    public function testContinuesAfterAnExplicitNonFatalFailure(): void
    {
        $runner = new SequenceRunner([
            new ProcessResult(9, '', 'optional hook failed'),
            new ProcessResult(0, '', 'build completed'),
        ]);
        $graph = new LifecycleGraph([
            new Step('optional.hook', Phase::Build, onFailure: FailureAction::Continue),
            new Step('build.application', Phase::Build, ['optional.hook']),
        ]);

        $result = (new LifecycleExecutor($runner, new RecordingCommandProvider()))->execute($graph, getcwd());

        self::assertTrue($result->succeeded());
        self::assertNull($result->failedStepId);
        self::assertSame(['optional.hook'], $result->continuedFailureStepIds);
        self::assertCount(2, $runner->requests);
    }
}

final class RecordingCommandProvider implements StepCommandProvider
{
    /** @var list<string> */
    public array $requestedStepIds = [];

    public function __construct(private readonly int $commandsPerStep = 1)
    {
    }

    public function commandsForStep(StepInterface $step): array
    {
        $this->requestedStepIds[] = $step->id();

        return array_map(
            static fn (int $index): CommandInterface => new Command(
                Executable::Composer,
                ['run', $step->id(), (string) $index],
            ),
            range(1, $this->commandsPerStep),
        );
    }
}

final class SequenceRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    /** @param list<ProcessResult> $results */
    public function __construct(private array $results)
    {
    }

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;
        $result = array_shift($this->results);
        if (!$result instanceof ProcessResult) {
            throw new RuntimeException('Runner received more requests than expected.');
        }

        return $result;
    }
}
