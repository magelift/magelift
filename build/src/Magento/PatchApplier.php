<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessRunner;
use RuntimeException;

/**
 * Clean-room m2-hotfixes discovery and fallback applicator.
 *
 * Behavioral contract from public Adobe Commerce Cloud docs: custom patches live
 * under project-root m2-hotfixes/*.patch and apply in alphabetical order via the
 * host patch(1) tool after composer install. Upstream Cloud Patches and Quality
 * Patches tools are selected by PatchLifecycle when the target project provides
 * them; this class does not vendor their source or patch databases.
 */
final class PatchApplier
{
    public const HOTFIX_DIRECTORY = 'm2-hotfixes';

    public function __construct(
        private ProcessRunner $runner,
        private string $patchBinary = 'patch',
    ) {
    }

    /**
     * Discover relative hotfix patch paths under the project root.
     *
     * Missing or empty m2-hotfixes directories yield an empty list (no-op).
     *
     * @return list<string> Paths relative to $projectRoot, sorted alphabetically by basename
     */
    public static function discover(string $projectRoot): array
    {
        $root = self::resolveProjectRoot($projectRoot);
        $hotfixDir = $root.DIRECTORY_SEPARATOR.self::HOTFIX_DIRECTORY;
        if (!is_dir($hotfixDir)) {
            return [];
        }

        $resolvedHotfix = realpath($hotfixDir);
        if ($resolvedHotfix === false || !self::isPathInside($resolvedHotfix, $root)) {
            throw new RuntimeException(sprintf(
                'Hotfix directory "%s" must resolve under the project root.',
                self::HOTFIX_DIRECTORY,
            ));
        }

        $entries = [];
        $handle = opendir($resolvedHotfix);
        if ($handle === false) {
            throw new RuntimeException(sprintf('Cannot read hotfix directory "%s".', self::HOTFIX_DIRECTORY));
        }
        try {
            while (($name = readdir($handle)) !== false) {
                if ($name === '.' || $name === '..') {
                    continue;
                }
                if (!str_ends_with($name, '.patch')) {
                    continue;
                }
                $candidate = $resolvedHotfix.DIRECTORY_SEPARATOR.$name;
                if (is_link($candidate)) {
                    throw new RuntimeException(sprintf(
                        'Hotfix patch "%s" must not be a symbolic link.',
                        self::HOTFIX_DIRECTORY.'/'.$name,
                    ));
                }
                if (!is_file($candidate)) {
                    continue;
                }
                $resolvedFile = realpath($candidate);
                if ($resolvedFile === false || !self::isPathInside($resolvedFile, $resolvedHotfix)) {
                    throw new RuntimeException(sprintf(
                        'Hotfix patch "%s" escapes the project hotfix directory.',
                        self::HOTFIX_DIRECTORY.'/'.$name,
                    ));
                }
                $entries[$name] = self::HOTFIX_DIRECTORY.'/'.$name;
            }
        } finally {
            closedir($handle);
        }

        ksort($entries, SORT_STRING);

        return array_values($entries);
    }

    /**
     * Build lifecycle commands that apply discovered patches with patch -p1.
     *
     * @param list<string> $relativePatchPaths
     * @return list<CommandInterface>
     */
    public static function commands(array $relativePatchPaths): array
    {
        $commands = [];
        foreach ($relativePatchPaths as $relative) {
            if (!is_string($relative)) {
                throw new RuntimeException('Hotfix patch paths must be strings.');
            }
            try {
                $commands[] = new PatchCommand($relative);
            } catch (\InvalidArgumentException $error) {
                throw new RuntimeException($error->getMessage(), previous: $error);
            }
        }

        return $commands;
    }

    /**
     * Discover and apply all m2-hotfixes under $projectRoot.
     *
     * Empty discovery is a successful no-op. Non-zero patch(1) exits fail loud.
     */
    public function apply(string $projectRoot, float $timeoutSeconds = 300.0): void
    {
        $patches = self::discover($projectRoot);
        if ($patches === []) {
            return;
        }
        $this->assertPatchToolAvailable();
        $root = self::resolveProjectRoot($projectRoot);
        foreach ($patches as $patchPath) {
            $command = new PatchCommand($patchPath, $this->patchBinary);
            $result = $command->run($this->runner, new ProcessRequest(
                $command->argv(),
                $root,
                [],
                $timeoutSeconds,
            ));
            if (!$result->succeeded()) {
                $patchPath = 'unknown';
                foreach ($command->arguments() as $argument) {
                    $patchPath = $argument;
                }
                throw new RuntimeException(sprintf(
                    'Failed to apply hotfix patch "%s" (exit %d).%s',
                    $patchPath,
                    $result->exitCode,
                    $result->stderr !== '' ? ' '.$result->stderr : '',
                ));
            }
        }
    }

    public function assertPatchToolAvailable(): void
    {
        $binary = $this->patchBinary;
        if ($binary !== 'patch' && is_executable($binary)) {
            return;
        }
        $path = getenv('PATH');
        if (!is_string($path) || $path === '') {
            throw new RuntimeException(
                'Required host tool "patch" was not found on PATH. Install GNU patch (or compatible) to apply m2-hotfixes.',
            );
        }
        foreach (explode(PATH_SEPARATOR, $path) as $dir) {
            if ($dir === '') {
                continue;
            }
            $candidate = $dir.DIRECTORY_SEPARATOR.$binary;
            if (is_file($candidate) && is_executable($candidate)) {
                return;
            }
        }

        throw new RuntimeException(
            'Required host tool "patch" was not found on PATH. Install GNU patch (or compatible) to apply m2-hotfixes.',
        );
    }

    private static function resolveProjectRoot(string $projectRoot): string
    {
        if ($projectRoot === '' || str_contains($projectRoot, "\0")) {
            throw new RuntimeException('Project root must be a non-empty path.');
        }
        $resolved = realpath($projectRoot);
        if ($resolved === false || !is_dir($resolved)) {
            throw new RuntimeException(sprintf('Project root "%s" does not exist.', $projectRoot));
        }

        return $resolved;
    }

    private static function isPathInside(string $path, string $root): bool
    {
        return $path === $root || str_starts_with($path, $root.DIRECTORY_SEPARATOR);
    }
}
