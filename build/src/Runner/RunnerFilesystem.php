<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

interface RunnerFilesystem
{
    public function checksum(string $path): string;

    public function checksumWithin(string $root, string $relativePath): string;

    public function read(string $path): string;

    public function atomicWrite(string $path, string $contents, int $permissions): void;
}
