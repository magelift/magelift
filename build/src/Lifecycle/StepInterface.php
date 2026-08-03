<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

interface StepInterface
{
    public function id(): string;

    public function phase(): Phase;

    /** @return list<string> */
    public function dependencies(): array;

    public function timeoutSeconds(): int;

    public function retryPolicy(): RetryPolicy;

    public function failureAction(): FailureAction;
}
