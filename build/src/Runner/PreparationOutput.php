<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

final readonly class PreparationOutput
{
    /**
     * @param list<string> $phpExtensions
     * @param list<string> $enabledModules
     * @param list<array{path: string, sha256: string}> $checksums
     * @param list<string> $requiredRuntimeCapabilities
     */
    public function __construct(
        public string $phpVersion,
        public array $phpExtensions,
        public array $enabledModules,
        public array $checksums,
        public array $requiredRuntimeCapabilities,
    ) {
    }
}
