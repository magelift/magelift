<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use InvalidArgumentException;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;

/**
 * Idempotent argv-only wrapper around the host patch(1) tool.
 *
 * A forward dry-run proves that the patch is still needed. If it fails, a
 * reverse dry-run proves that the patch is already present. No shell is used,
 * and the real patch command is only executed after the forward dry-run.
 */
final readonly class PatchCommand implements CommandInterface
{
    /** @var list<string> */
    private array $patchArguments;

    public function __construct(
        private string $patchPath,
        private string $patchBinary = 'patch',
    ) {
        self::assertPatchPath($patchPath);
        if ($patchBinary === '' || str_contains($patchBinary, "\0")) {
            throw new InvalidArgumentException('Patch executable must be a non-empty string without null bytes.');
        }
        $this->patchArguments = [
            '-p1',
            '--forward',
            '--batch',
            '-i',
            $patchPath,
        ];
    }

    public function executable(): Executable
    {
        return Executable::Patch;
    }

    public function arguments(): array
    {
        return $this->patchArguments;
    }

    public function argv(): array
    {
        return [$this->patchBinary, ...$this->patchArguments];
    }

    public function run(ProcessRunner $runner, ProcessRequest $request): ProcessResult
    {
        $forwardDryRun = $runner->run($this->requestWithArguments(
            $request,
            ['-p1', '--forward', '--batch', '--dry-run', '-i', $this->patchPath],
        ));
        if ($forwardDryRun->succeeded()) {
            return $runner->run($request);
        }
        if ($forwardDryRun->timedOut || $forwardDryRun->cancelled) {
            return $forwardDryRun;
        }

        $reverseDryRun = $runner->run($this->requestWithArguments(
            $request,
            ['-p1', '--reverse', '--batch', '--dry-run', '-i', $this->patchPath],
        ));
        if ($reverseDryRun->succeeded()) {
            return new ProcessResult(
                0,
                trim($forwardDryRun->stdout."\nPatch already applied: ".$this->patchPath),
                $forwardDryRun->stderr,
            );
        }

        return new ProcessResult(
            $forwardDryRun->exitCode,
            $forwardDryRun->stdout,
            self::joinDiagnostics($forwardDryRun->stderr, $reverseDryRun->stderr),
            $reverseDryRun->timedOut,
            $reverseDryRun->cancelled,
        );
    }

    public static function assertPatchPath(string $patchPath): void
    {
        if (
            $patchPath === ''
            || str_contains($patchPath, "\0")
            || !str_starts_with($patchPath, PatchApplier::HOTFIX_DIRECTORY.'/')
            || str_contains($patchPath, '..')
            || str_contains($patchPath, '\\')
        ) {
            throw new InvalidArgumentException(sprintf(
                'Hotfix patch path "%s" must stay under %s/.',
                $patchPath,
                PatchApplier::HOTFIX_DIRECTORY,
            ));
        }
    }

    /** @param list<string> $arguments */
    private function requestWithArguments(ProcessRequest $request, array $arguments): ProcessRequest
    {
        return new ProcessRequest(
            [$this->patchBinary, ...$arguments],
            $request->workingDirectory,
            $request->environment,
            $request->timeoutSeconds,
            $request->isCancelled,
        );
    }

    private static function joinDiagnostics(string $first, string $second): string
    {
        $diagnostics = array_values(array_filter([$first, $second], static fn (string $value): bool => $value !== ''));

        return implode("\n", $diagnostics);
    }
}
