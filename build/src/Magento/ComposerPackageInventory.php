<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use JsonException;
use RuntimeException;

/**
 * Reads only production package names from Composer inputs.
 */
final class ComposerPackageInventory
{
    /** @return list<string> */
    public static function read(string $projectRoot): array
    {
        $root = realpath($projectRoot);
        if ($root === false || !is_dir($root)) {
            throw new RuntimeException(sprintf('Project root "%s" does not exist.', $projectRoot));
        }

        $packages = [];
        $manifestPath = $root.'/composer.json';
        if (is_file($manifestPath)) {
            $manifest = self::readJson($manifestPath, 'Composer manifest');
            self::readNameMap($manifest['require'] ?? null, $packages, 'composer.json require');
        }

        $lockPath = $root.'/composer.lock';
        if (is_file($lockPath)) {
            $lock = self::readJson($lockPath, 'Composer lock');
            self::readPackageList($lock['packages'] ?? null, $packages, 'composer.lock packages');
        }

        $names = array_keys($packages);
        sort($names, SORT_STRING);

        return $names;
    }

    /** @param array<string, true> $packages */
    private static function readNameMap(mixed $value, array &$packages, string $field): void
    {
        if ($value === null) {
            return;
        }
        if (!is_array($value)) {
            throw new RuntimeException(sprintf('%s must be an object.', $field));
        }
        foreach ($value as $name => $_constraint) {
            if (!is_string($name) || $name === '') {
                throw new RuntimeException(sprintf('%s contains an invalid package name.', $field));
            }
            $packages[$name] = true;
        }
    }

    /** @param array<string, true> $packages */
    private static function readPackageList(mixed $value, array &$packages, string $field): void
    {
        if ($value === null) {
            return;
        }
        if (!is_array($value)) {
            throw new RuntimeException(sprintf('%s must be a list.', $field));
        }
        foreach ($value as $package) {
            if (!is_array($package) || !isset($package['name']) || !is_string($package['name']) || $package['name'] === '') {
                throw new RuntimeException(sprintf('%s contains a package without a valid name.', $field));
            }
            $packages[$package['name']] = true;
        }
    }

    /** @return array<string, mixed> */
    private static function readJson(string $path, string $label): array
    {
        $contents = @file_get_contents($path);
        if ($contents === false) {
            throw new RuntimeException(sprintf('Cannot read %s at "%s".', $label, $path));
        }
        try {
            $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $error) {
            throw new RuntimeException(sprintf('Invalid JSON in %s at "%s".', $label, $path), previous: $error);
        }
        if (!is_array($decoded)) {
            throw new RuntimeException(sprintf('%s at "%s" must contain a JSON object.', $label, $path));
        }

        return $decoded;
    }
}
