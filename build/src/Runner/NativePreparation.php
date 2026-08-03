<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use FilesystemIterator;
use MageLift\Build\Lifecycle\LifecycleExecutor;
use MageLift\Build\Lifecycle\LifecycleGraph;
use MageLift\Build\Lifecycle\LifecycleHook;
use MageLift\Build\Lifecycle\FailureAction;
use MageLift\Build\Lifecycle\HookRelationship;
use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\RetryPolicy;
use MageLift\Build\Lifecycle\Step;
use MageLift\Build\Magento\LifecyclePlan;
use MageLift\Build\Magento\Command;
use MageLift\Build\Magento\Executable;
use MageLift\Build\Magento\PatchApplier;
use MageLift\Build\Process\ProcessRunner;
use RecursiveDirectoryIterator;
use RecursiveCallbackFilterIterator;
use RecursiveIteratorIterator;
use RuntimeException;

final readonly class NativePreparation implements Preparation
{
    public function __construct(
        private ProcessRunner $runner,
        private string $workspaceRoot,
        private RequiredCapabilityProvider $capabilities,
        private ConfigModuleReader $modules,
    ) {
    }

    public function prepare(\MageLift\Build\Protocol\PrepareRequest $request): PreparationOutput
    {
        if (!$this->matchesPHPBranch($request->phpVersion)) {
            throw new RuntimeException('Runner PHP version does not match the requested PHP branch.');
        }
        $buildRoot = rtrim($this->workspaceRoot, '/').'/rootfs';
        $this->copySource($request->repositoryRoot, $buildRoot);

        $hotfixPatches = PatchApplier::discover($buildRoot);
        if ($hotfixPatches !== []) {
            (new PatchApplier($this->runner))->assertPatchToolAvailable();
        }
        $plan = new LifecyclePlan(
            $this->hookCommands($request->lifecycleHooks),
            $request->staticContent,
            $hotfixPatches,
        );
        $execution = (new LifecycleExecutor($this->runner, $plan))->execute(
            new LifecycleGraph($plan->steps(), $this->lifecycleHooks($request->lifecycleHooks)),
            $buildRoot,
            $this->environment(),
        );
        if (!$execution->succeeded()) {
            $failed = $execution->steps !== [] ? $execution->steps[count($execution->steps) - 1] : null;
            $lastAttempt = $failed !== null && $failed->attempts !== [] ? $failed->attempts[count($failed->attempts) - 1] : [];
            $lastProcess = $lastAttempt !== [] ? $lastAttempt[count($lastAttempt) - 1] : null;
            $exitCode = $lastProcess !== null ? $lastProcess->exitCode : 1;
            $phase = $failed?->phase->value ?? Phase::Build->value;
            throw new RuntimeException(sprintf(
                'Lifecycle preparation failed during %s step %s (exit %d).',
                $phase,
                $execution->failedStepId ?? 'unknown',
                $exitCode,
            ));
        }

        $checksums = [];
        foreach (['app/etc/config.php', 'vendor/autoload.php'] as $path) {
            $checksum = @hash_file('sha256', $buildRoot.'/'.$path);
            if ($checksum === false) {
                throw new RuntimeException('Prepared output is missing a required file.');
            }
            $checksums[] = ['path' => $path, 'sha256' => $checksum];
        }
        $extensions = array_values(array_unique(array_map(
            static fn (string $extension): string => preg_replace('/[^a-z0-9_-]+/', '_', strtolower($extension)) ?? '',
            get_loaded_extensions(),
        )));

        return new PreparationOutput(
            PHP_VERSION,
            $extensions,
            $this->modules->enabledModules($buildRoot.'/app/etc/config.php'),
            $checksums,
            $this->capabilities->capabilitiesFor($request),
        );
    }

    /** @return array<string, string> */
    private function environment(): array
    {
        $environment = [];
        foreach (['PATH', 'HOME'] as $name) {
            $value = getenv($name);
            if (is_string($value) && $value !== '') {
                $environment[$name] = $value;
            }
        }
        if (is_file('/run/secrets/composer-auth')) {
            $composerAuth = @file_get_contents('/run/secrets/composer-auth');
            if ($composerAuth === false || trim($composerAuth) === '') {
                throw new RuntimeException('Composer authentication secret cannot be read.');
            }
            $environment['COMPOSER_AUTH'] = trim($composerAuth);
        }

        return $environment;
    }

    /**
     * @param list<array<string, mixed>> $hooks
     * @return list<LifecycleHook>
     */
    private function lifecycleHooks(array $hooks): array
    {
        $result = [];
        foreach ($hooks as $hook) {
            if ($hook['relationship'] === HookRelationship::Disable->value) {
                $result[] = new LifecycleHook(HookRelationship::Disable, $hook['target']);
                continue;
            }
            $result[] = new LifecycleHook(
                HookRelationship::from($hook['relationship']),
                $hook['target'],
                new Step(
                    $hook['id'],
                    Phase::from($hook['phase']),
                    $hook['dependencies'],
                    $hook['timeoutSeconds'],
                    new RetryPolicy(
                        $hook['retries']['maxAttempts'],
                        $hook['retries']['delaySeconds'],
                        $hook['retries']['idempotent'],
                    ),
                    FailureAction::from($hook['failure']),
                ),
            );
        }

        return $result;
    }

    /**
     * @param list<array<string, mixed>> $hooks
     * @return array<string, list<\MageLift\Build\Magento\CommandInterface>>
     */
    private function hookCommands(array $hooks): array
    {
        $commands = [];
        foreach ($hooks as $hook) {
            if ($hook['command'] === null) {
                continue;
            }
            $commands[$hook['id']] = [new Command(
                Executable::from($hook['command']['executable']),
                $hook['command']['arguments'],
            )];
        }

        return $commands;
    }

    private function matchesPHPBranch(string $requested): bool
    {
        if (preg_match('/^\d+\.\d+(?:\.\d+)?$/D', $requested) !== 1) {
            return false;
        }

        return PHP_VERSION === $requested || str_starts_with(PHP_VERSION, $requested.'.');
    }

    private function copySource(string $source, string $destination): void
    {
        foreach (['auth.json', 'app/etc/env.php'] as $sensitivePath) {
            $path = rtrim($source, '/').'/'.$sensitivePath;
            if (file_exists($path) || is_link($path)) {
                throw new RuntimeException(sprintf('Sensitive source file %s cannot enter the build workspace.', $sensitivePath));
            }
        }
        if (file_exists($destination)) {
            throw new RuntimeException('Workspace build root already exists.');
        }
        if (!@mkdir($destination, 0o700, true)) {
            throw new RuntimeException('Cannot create workspace build root.');
        }
        $sourceIterator = new RecursiveDirectoryIterator($source, FilesystemIterator::SKIP_DOTS);
        $filteredIterator = new RecursiveCallbackFilterIterator(
            $sourceIterator,
            static function ($entry, $_key, RecursiveDirectoryIterator $iterator): bool {
                $relativePath = $iterator->getSubPathName();
                return $relativePath !== '.git'
                    && $relativePath !== '.magelift'
                    && $relativePath !== 'magelift.yaml';
            },
        );
        $iterator = new RecursiveIteratorIterator(
            $filteredIterator,
            RecursiveIteratorIterator::SELF_FIRST,
        );
        foreach ($iterator as $entry) {
            if ($entry->isLink()) {
                throw new RuntimeException('Source repository cannot contain symbolic links during preparation.');
            }
            $relative = $iterator->getSubPathName();
            $target = $destination.'/'.$relative;
            if ($entry->isDir()) {
                if (!@mkdir($target, 0o700) && !is_dir($target)) {
                    throw new RuntimeException('Cannot copy source directory into workspace.');
                }
            } elseif (!@copy($entry->getPathname(), $target)) {
                throw new RuntimeException('Cannot copy source file into workspace.');
            }
            if (!@chmod($target, $entry->getPerms() & 0o777)) {
                throw new RuntimeException('Cannot preserve source permissions in workspace.');
            }
        }
    }
}
