<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

use Closure;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessRunner;

final readonly class LifecycleExecutor
{
    /** @var Closure(int): void */
    private Closure $sleep;

    /** @param null|Closure(int): void $sleep */
    public function __construct(
        private ProcessRunner $runner,
        private StepCommandProvider $commands,
        ?Closure $sleep = null,
    ) {
        $this->sleep = $sleep ?? static function (int $seconds): void {
            sleep($seconds);
        };
    }

    /**
     * @param array<string, string> $environment
     * @param null|Closure(): bool $isCancelled
     */
    public function execute(
        LifecycleGraph $graph,
        string $workingDirectory,
        array $environment = [],
        ?Closure $isCancelled = null,
    ): LifecycleExecutionResult {
        $stepResults = [];
        $continuedFailures = [];
        foreach ($graph->orderedSteps() as $step) {
            $result = $this->executeStep($step, $workingDirectory, $environment, $isCancelled);
            $stepResults[] = $result;
            if (!$result->succeeded()) {
                if ($step->failureAction() === FailureAction::Continue) {
                    $continuedFailures[] = $step->id();
                    continue;
                }

                return new LifecycleExecutionResult($stepResults, $step->id(), $continuedFailures);
            }
        }

        return new LifecycleExecutionResult($stepResults, continuedFailureStepIds: $continuedFailures);
    }

    /** @param array<string, string> $environment */
    private function executeStep(
        StepInterface $step,
        string $workingDirectory,
        array $environment,
        ?Closure $isCancelled,
    ): StepExecutionResult {
        $attempts = [];
        $policy = $step->retryPolicy();
        $commands = $this->commands->commandsForStep($step);
        for ($attempt = 1; $attempt <= $policy->maxAttempts; ++$attempt) {
            $processResults = [];
            foreach ($commands as $command) {
                $processResult = $this->runner->run(new ProcessRequest(
                    $command->argv(),
                    $workingDirectory,
                    $environment,
                    $step->timeoutSeconds(),
                    $isCancelled,
                ));
                $processResults[] = $processResult;
                if (!$processResult->succeeded()) {
                    break;
                }
            }
            $attempts[] = $processResults;

            if (
                $this->attemptSucceeded($processResults)
                || $this->attemptWasCancelled($processResults)
                || $attempt === $policy->maxAttempts
            ) {
                break;
            }
            if ($policy->delaySeconds > 0) {
                ($this->sleep)($policy->delaySeconds);
            }
        }
		if ($attempts === []) {
			throw new \LogicException('Lifecycle step execution produced no attempts.');
		}

        return new StepExecutionResult($step->id(), $step->phase(), $attempts);
    }

    /** @param list<\MageLift\Build\Process\ProcessResult> $results */
    private function attemptSucceeded(array $results): bool
    {
        foreach ($results as $result) {
            if (!$result->succeeded()) {
                return false;
            }
        }

        return true;
    }

    /** @param list<\MageLift\Build\Process\ProcessResult> $results */
    private function attemptWasCancelled(array $results): bool
    {
        foreach ($results as $result) {
            if ($result->cancelled) {
                return true;
            }
        }

        return false;
    }
}
