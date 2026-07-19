<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use RuntimeException;

final class NativeRunnerFilesystem implements RunnerFilesystem
{
    public function checksum(string $path): string
    {
        $checksum = @hash_file('sha256', $path);
        if ($checksum === false) {
            throw new RuntimeException('Cannot checksum required file.');
        }

        return $checksum;
    }

    public function checksumWithin(string $root, string $relativePath): string
    {
        $resolvedRoot = realpath($root);
        $resolvedFile = realpath($root.DIRECTORY_SEPARATOR.$relativePath);
        if (
            $resolvedRoot === false
            || $resolvedFile === false
            || ($resolvedFile !== $resolvedRoot && !str_starts_with($resolvedFile, $resolvedRoot.DIRECTORY_SEPARATOR))
        ) {
            throw new RuntimeException('Immutable input is missing or escapes the repository root.');
        }

        return $this->checksum($resolvedFile);
    }

    public function read(string $path): string
    {
        $contents = @file_get_contents($path);
        if ($contents === false) {
            throw new RuntimeException('Cannot read prepared metadata.');
        }

        return $contents;
    }

    public function atomicWrite(string $path, string $contents, int $permissions): void
    {
        $directory = dirname($path);
        if (!is_dir($directory) && !@mkdir($directory, 0o700, true) && !is_dir($directory)) {
            throw new RuntimeException('Cannot create runner output directory.');
        }
        $temporary = @tempnam($directory, '.magelift-');
        if ($temporary === false) {
            throw new RuntimeException('Cannot create temporary runner output.');
        }
        try {
            if (@file_put_contents($temporary, $contents, LOCK_EX) === false || !@chmod($temporary, $permissions)) {
                throw new RuntimeException('Cannot write runner output.');
            }
            if (!@rename($temporary, $path)) {
                throw new RuntimeException('Cannot atomically publish runner output.');
            }
        } finally {
            if (is_file($temporary)) {
                @unlink($temporary);
            }
        }
    }
}
