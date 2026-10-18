<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

use JsonException;

final class ProtocolJson
{
    public const VERSION = 1;
    public const MAX_BYTES = 1_048_576;

    /** @return array<string, mixed> */
    public static function decodeObject(string $json): array
    {
        if (strlen($json) > self::MAX_BYTES) {
            throw new InvalidProtocolRequest('Protocol message exceeds the 1048576-byte limit.');
        }
        try {
            $value = json_decode($json, true, 32, JSON_THROW_ON_ERROR);
        } catch (JsonException $error) {
            throw new InvalidProtocolRequest('Protocol message is not valid JSON.', previous: $error);
        }
        if (!is_array($value) || array_is_list($value)) {
            throw new InvalidProtocolRequest('Protocol message must be a JSON object.');
        }

        return $value;
    }

    /**
     * @param array<string, mixed> $value
     * @param list<string> $allowed
     * @param list<string> $required
     */
    public static function assertKeys(array $value, array $allowed, array $required, string $path = 'request'): void
    {
        $unknown = array_diff(array_keys($value), $allowed);
        if ($unknown !== []) {
            throw new InvalidProtocolRequest(sprintf('%s contains unknown key "%s".', $path, array_values($unknown)[0]));
        }
        $missing = array_diff($required, array_keys($value));
        if ($missing !== []) {
            throw new InvalidProtocolRequest(sprintf('%s is missing required key "%s".', $path, array_values($missing)[0]));
        }
    }

    /** @param array<string, mixed> $value */
    public static function assertEnvelope(array $value, ProtocolOperation $stage): void
    {
        if (($value['protocolVersion'] ?? null) !== self::VERSION) {
            throw new InvalidProtocolRequest('Unsupported protocolVersion; expected 1.');
        }
        if (($value['stage'] ?? null) !== $stage->value) {
            throw new InvalidProtocolRequest(sprintf('Protocol stage must be "%s".', $stage->value));
        }
    }

	/** @param array<string, mixed> $value */
	public static function encode(array $value): string
    {
        self::assertNoSecrets($value);
        try {
            $json = json_encode($value, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);
        } catch (JsonException $error) {
            throw new ProtocolException('Protocol message cannot be encoded as JSON.', previous: $error);
        }
        $json = str_replace(['<', '>', '&'], ['\u003c', '\u003e', '\u0026'], $json);
        if (strlen($json) > self::MAX_BYTES) {
            throw new ProtocolException('Protocol message exceeds the 1048576-byte limit.');
        }

        return $json;
    }

    /** @return array<string, mixed> */
    public static function object(mixed $value, string $field): array
    {
        if (!is_array($value) || array_is_list($value)) {
            throw new InvalidProtocolRequest(sprintf('%s must be an object.', $field));
        }

        return $value;
    }

    /** @return list<mixed> */
    public static function list(mixed $value, string $field): array
    {
        if (!is_array($value) || !array_is_list($value)) {
            throw new InvalidProtocolRequest(sprintf('%s must be a list.', $field));
        }

        return $value;
    }

    public static function string(mixed $value, string $field): string
    {
        if (!is_string($value)) {
            throw new InvalidProtocolRequest(sprintf('%s must be a string.', $field));
        }

        return $value;
    }

    public static function nonEmptyString(mixed $value, string $field): string
    {
        $string = self::string($value, $field);
        if (preg_match('/^\s*$/u', $string) === 1) {
            throw new InvalidProtocolRequest(sprintf('%s is required.', $field));
        }

        return $string;
    }

    public static function revision(mixed $value): string
    {
        $revision = self::string($value, 'sourceRevision');
        if (preg_match('/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/D', $revision) !== 1) {
            throw new InvalidProtocolRequest('sourceRevision must be a lowercase commit hash.');
        }

        return $revision;
    }

    public static function digest(mixed $value): string
    {
        $digest = self::string($value, 'imageDigest');
        if (preg_match('/^sha256:[a-f0-9]{64}$/D', $digest) !== 1) {
            throw new InvalidProtocolRequest('imageDigest must be a lowercase SHA-256 OCI digest.');
        }

        return $digest;
    }

    public static function checksum(mixed $value, string $name): string
    {
        $checksum = self::string($value, $name.' checksum');
        if (preg_match('/^[a-f0-9]{64}$/D', $checksum) !== 1) {
            throw new InvalidProtocolRequest(sprintf('%s must be a lowercase SHA-256 checksum.', $name));
        }

        return $checksum;
    }

    public static function relativePath(mixed $value, string $name): string
    {
        $path = self::string($value, $name.' path');
        $segments = explode('/', $path);
        $clean = [];
        $traversesParent = false;
        foreach ($segments as $segment) {
            if ($segment === '' || $segment === '.') {
                continue;
            }
            if ($segment === '..') {
                if ($clean === []) {
                    $traversesParent = true;
                } else {
                    array_pop($clean);
                }
                continue;
            }
            $clean[] = $segment;
        }
        $cleanPath = implode('/', $clean);
        if (
            $path === ''
            || str_contains($path, '\\')
            || str_starts_with($path, '/')
            || $traversesParent
            || $cleanPath === ''
            || $cleanPath === '..'
            || str_starts_with($cleanPath, '../')
            || str_starts_with($path, '../')
        ) {
            throw new InvalidProtocolRequest(sprintf('%s must be a portable relative path and cannot traverse its parent.', $name));
        }

        return $path;
    }

    public static function assertNoSecrets(mixed $value, string $key = '', int $depth = 0): void
    {
        if ($depth > 32) {
            throw new ProtocolException('Protocol data exceeds the maximum nesting depth.');
        }
        if ($key !== '' && preg_match('/secret|password|token|credential|authorization|private.?key/i', $key) === 1) {
            throw new ProtocolException('Protocol data contains a forbidden secret field.');
        }
        if (is_string($value) && (
            preg_match('/^(?:aws-secrets-manager|ssm|vault):\/\//i', $value) === 1
            || str_contains($value, '-----BEGIN PRIVATE KEY-----')
        )) {
            throw new ProtocolException('Protocol data contains a forbidden secret value.');
        }
        if (!is_array($value)) {
            return;
        }
        foreach ($value as $childKey => $child) {
            self::assertNoSecrets($child, is_string($childKey) ? $childKey : '', $depth + 1);
        }
    }
}
