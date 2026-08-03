<?php

declare(strict_types=1);

namespace MageLift\Build\Artifact;

use RuntimeException;

final class ArtifactManifestWriter
{
    public function write(ArtifactManifest $manifest, string $path): void
    {
        $directory = dirname($path);
        if (!is_dir($directory)) {
            throw new RuntimeException(sprintf('Artifact manifest directory "%s" does not exist.', $directory));
        }

        $temporaryPath = tempnam($directory, '.magelift-manifest-');
        if ($temporaryPath === false) {
            throw new RuntimeException(sprintf('Cannot create a temporary artifact manifest in "%s".', $directory));
        }

        try {
            $contents = $manifest->toCanonicalJson()."\n";
            if (file_put_contents($temporaryPath, $contents, LOCK_EX) !== strlen($contents)) {
                throw new RuntimeException('Cannot write the complete artifact manifest.');
            }
            if (!chmod($temporaryPath, 0644)) {
                throw new RuntimeException('Cannot set artifact manifest permissions.');
            }
            if (!rename($temporaryPath, $path)) {
                throw new RuntimeException(sprintf('Cannot replace artifact manifest "%s".', $path));
            }
        } finally {
            if (is_file($temporaryPath)) {
                @unlink($temporaryPath);
            }
        }
    }
}
