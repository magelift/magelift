<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

interface RevisionVerifier
{
    public function matches(string $repositoryRoot, string $sourceRevision): bool;
}
