<?php

declare(strict_types=1);

namespace MageLift\Build\Process;

use Closure;
use InvalidArgumentException;

final readonly class ProcessRequest
{
    /**
     * @param non-empty-list<string> $argv
     * @param array<string, string> $environment
     * @param null|Closure(): bool $isCancelled
     */
    public function __construct(
        public array $argv,
        public string $workingDirectory,
        public array $environment = [],
        public float $timeoutSeconds = 300.0,
        public ?Closure $isCancelled = null,
    ) {
        if ($this->argv === []) {
            throw new InvalidArgumentException('Process argument vector cannot be empty.');
        }
        foreach ($this->argv as $argument) {
            if (!is_string($argument) || $argument === '' || str_contains($argument, "\0")) {
                throw new InvalidArgumentException('Process arguments must be non-empty strings without null bytes.');
            }
        }
        if (!is_dir($this->workingDirectory)) {
            throw new InvalidArgumentException(sprintf('Process working directory "%s" does not exist.', $this->workingDirectory));
        }
        if ($this->timeoutSeconds <= 0) {
            throw new InvalidArgumentException('Process timeout must be greater than zero seconds.');
        }
        foreach ($this->environment as $name => $value) {
            if (preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/D', $name) !== 1 || str_contains($value, "\0")) {
                throw new InvalidArgumentException(sprintf('Invalid process environment entry "%s".', $name));
            }
        }
    }
}
