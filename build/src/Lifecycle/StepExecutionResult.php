<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

use MageLift\Build\Process\ProcessResult;

final readonly class StepExecutionResult
{
    /** @param non-empty-list<list<ProcessResult>> $attempts */
    public function __construct(
        public string $stepId,
        public Phase $phase,
        public array $attempts,
    ) {
    }

    public function succeeded(): bool
    {
        $lastAttempt = $this->attempts[array_key_last($this->attempts)];
        foreach ($lastAttempt as $result) {
            if (!$result->succeeded()) {
                return false;
            }
        }

        return true;
    }
}
