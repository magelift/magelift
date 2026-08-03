<?php

declare(strict_types=1);

namespace MageLift\Build\Process;

final readonly class ProcessResult
{
    public function __construct(
        public int $exitCode,
        public string $stdout,
        public string $stderr,
        public bool $timedOut = false,
        public bool $cancelled = false,
    ) {
    }

    public function succeeded(): bool
    {
        return $this->exitCode === 0 && !$this->timedOut && !$this->cancelled;
    }
}
