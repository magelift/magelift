<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

/** @internal */
final readonly class ConfiguredStep implements StepInterface
{
    /** @param list<string> $dependencies */
    public function __construct(
        private StepInterface $step,
        private array $dependencies,
    ) {
    }

    public function id(): string
    {
        return $this->step->id();
    }

    public function phase(): Phase
    {
        return $this->step->phase();
    }

    public function dependencies(): array
    {
        return $this->dependencies;
    }

    public function timeoutSeconds(): int
    {
        return $this->step->timeoutSeconds();
    }

    public function retryPolicy(): RetryPolicy
    {
        return $this->step->retryPolicy();
    }

    public function failureAction(): FailureAction
    {
        return $this->step->failureAction();
    }
}
