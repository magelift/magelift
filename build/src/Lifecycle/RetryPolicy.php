<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

use InvalidArgumentException;

final readonly class RetryPolicy
{
    public function __construct(
        public int $maxAttempts = 1,
        public int $delaySeconds = 0,
        public bool $idempotent = false,
    ) {
        if ($this->maxAttempts < 1) {
            throw new InvalidArgumentException('Retry attempts must be at least one.');
        }

        if ($this->delaySeconds < 0) {
            throw new InvalidArgumentException('Retry delay cannot be negative.');
        }

        if ($this->maxAttempts > 1 && !$this->idempotent) {
            throw new InvalidArgumentException('Only idempotent steps may be retried.');
        }
    }
}
