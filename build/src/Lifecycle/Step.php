<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

use InvalidArgumentException;

final readonly class Step implements StepInterface
{
    private const ID_PATTERN = '/^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$/D';

    /** @var list<string> */
    private array $dependencyIds;

    /**
     * @param list<string> $dependencies
     */
    public function __construct(
        private string $stepId,
        private Phase $stepPhase,
        array $dependencies = [],
        private int $timeout = 300,
        private RetryPolicy $retries = new RetryPolicy(),
        private FailureAction $onFailure = FailureAction::Abort,
    ) {
        self::assertValidId($this->stepId);

        if ($this->timeout < 1) {
            throw new InvalidArgumentException('Step timeout must be at least one second.');
        }

        if (count($dependencies) !== count(array_unique($dependencies))) {
            throw new InvalidArgumentException(sprintf('Step "%s" contains duplicate dependencies.', $this->stepId));
        }

        foreach ($dependencies as $dependency) {
            self::assertValidId($dependency);
        }

        $this->dependencyIds = array_values($dependencies);
    }

    public function id(): string
    {
        return $this->stepId;
    }

    public function phase(): Phase
    {
        return $this->stepPhase;
    }

    public function dependencies(): array
    {
        return $this->dependencyIds;
    }

    public function timeoutSeconds(): int
    {
        return $this->timeout;
    }

    public function retryPolicy(): RetryPolicy
    {
        return $this->retries;
    }

    public function failureAction(): FailureAction
    {
        return $this->onFailure;
    }

    private static function assertValidId(string $id): void
    {
        if (preg_match(self::ID_PATTERN, $id) !== 1) {
            throw new InvalidArgumentException(sprintf('Invalid stable step ID "%s".', $id));
        }
    }
}
