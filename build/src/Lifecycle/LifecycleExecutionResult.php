<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

final readonly class LifecycleExecutionResult
{
    /** @param list<StepExecutionResult> $steps */
    public function __construct(
        public array $steps,
        public ?string $failedStepId = null,
        /** @var list<string> */
        public array $continuedFailureStepIds = [],
    ) {
    }

    public function succeeded(): bool
    {
        return $this->failedStepId === null;
    }
}
