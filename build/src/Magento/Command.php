<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use InvalidArgumentException;

final readonly class Command implements CommandInterface
{
    /** @var list<string> */
    private array $commandArguments;

    /** @param list<string> $arguments */
    public function __construct(
        private Executable $commandExecutable,
        array $arguments,
    ) {
        foreach ($arguments as $argument) {
            if (!is_string($argument) || $argument === '' || str_contains($argument, "\0")) {
                throw new InvalidArgumentException('Command arguments must be non-empty strings without null bytes.');
            }
        }

        $this->commandArguments = array_values($arguments);
    }

    public function executable(): Executable
    {
        return $this->commandExecutable;
    }

    public function arguments(): array
    {
        return $this->commandArguments;
    }

    public function argv(): array
    {
        return [$this->commandExecutable->value, ...$this->commandArguments];
    }
}
