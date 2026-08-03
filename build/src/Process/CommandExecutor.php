<?php

declare(strict_types=1);

namespace MageLift\Build\Process;

use Closure;
use InvalidArgumentException;
use MageLift\Build\Magento\CommandInterface;

final readonly class CommandExecutor
{
    /** @var list<string> */
    private array $environmentAllowlist;

    /** @var list<string> */
    private array $workingDirectoryAllowlist;

    /**
     * @param list<string> $environmentAllowlist
     * @param list<string> $workingDirectoryAllowlist
     */
    public function __construct(
        private ProcessRunner $runner,
        array $environmentAllowlist = [],
        array $workingDirectoryAllowlist = [],
    ) {
        foreach ($environmentAllowlist as $name) {
            if (preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/D', $name) !== 1) {
                throw new InvalidArgumentException(sprintf('Invalid allowed environment variable "%s".', $name));
            }
        }
        $this->environmentAllowlist = array_values(array_unique($environmentAllowlist));
        $currentDirectory = getcwd();
        if ($workingDirectoryAllowlist === [] && $currentDirectory === false) {
            throw new InvalidArgumentException('Cannot determine the current working directory.');
        }
        $roots = $workingDirectoryAllowlist === [] ? [$currentDirectory] : $workingDirectoryAllowlist;
        $this->workingDirectoryAllowlist = array_map(
            static function (string $root): string {
                $resolved = realpath($root);
                if ($resolved === false || !is_dir($resolved)) {
                    throw new InvalidArgumentException(sprintf('Allowed working directory "%s" does not exist.', $root));
                }

                return $resolved;
            },
            $roots,
        );
    }

    /**
     * @param array<string, string> $environment
     * @param null|Closure(): bool $isCancelled
     */
    public function execute(
        CommandInterface $command,
        string $workingDirectory,
        array $environment = [],
        float $timeoutSeconds = 300.0,
        ?Closure $isCancelled = null,
    ): ProcessResult {
        $resolvedDirectory = realpath($workingDirectory);
        if ($resolvedDirectory === false || !$this->isWorkingDirectoryAllowed($resolvedDirectory)) {
            throw new InvalidArgumentException(sprintf('Working directory "%s" is not allowed.', $workingDirectory));
        }
        $unknownVariables = array_diff(array_keys($environment), $this->environmentAllowlist);
        if ($unknownVariables !== []) {
            throw new InvalidArgumentException(sprintf(
                'Process environment variable "%s" is not allowed.',
                array_values($unknownVariables)[0],
            ));
        }

        return $this->runner->run(new ProcessRequest(
            $command->argv(),
            $resolvedDirectory,
            $environment,
            $timeoutSeconds,
            $isCancelled,
        ));
    }

    /**
     * @param array<string, string> $environment
     * @param null|Closure(): bool $isCancelled
     */
    public function executeOrFail(
        CommandInterface $command,
        string $workingDirectory,
        array $environment = [],
        float $timeoutSeconds = 300.0,
        ?Closure $isCancelled = null,
    ): ProcessResult {
        $result = $this->execute($command, $workingDirectory, $environment, $timeoutSeconds, $isCancelled);
        if (!$result->succeeded()) {
            throw new ProcessFailed($result);
        }

        return $result;
    }

    private function isWorkingDirectoryAllowed(string $directory): bool
    {
        foreach ($this->workingDirectoryAllowlist as $root) {
            if ($directory === $root || str_starts_with($directory, $root.DIRECTORY_SEPARATOR)) {
                return true;
            }
        }

        return false;
    }
}
