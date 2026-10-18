<?php

declare(strict_types=1);

namespace MageLift\Build\Artifact;

use InvalidArgumentException;

final readonly class BuildRecipe
{
    private const CHECKSUM_PATTERN = '/^[a-f0-9]{64}$/D';
    private const PLATFORM_PATTERN = '/^linux\/(?:amd64|arm64)$/D';
    private const PINNED_DIGEST_PATTERN = '/^[^\s@]+@sha256:[a-f0-9]{64}$/D';

    /** @var list<string> */
    private const BASE_IMAGE_ROLES = ['COMPOSER_BASE', 'NGINX_BASE', 'PHP_BASE'];

    /**
     * @param array<string, string> $baseImages
     */
    public function __construct(
        public string $sha256,
        public string $platform,
        public array $baseImages,
    ) {
        self::assertMatches($this->sha256, self::CHECKSUM_PATTERN, 'Build recipe sha256 must be a lowercase SHA-256 checksum.');
        self::assertMatches($this->platform, self::PLATFORM_PATTERN, 'Build recipe platform must be linux/amd64 or linux/arm64.');
        if ($this->baseImages === []) {
            throw new InvalidArgumentException('Build recipe baseImages cannot be empty.');
        }
        $seen = [];
        foreach ($this->baseImages as $role => $reference) {
            if (!is_string($role) || !in_array($role, self::BASE_IMAGE_ROLES, true)) {
                throw new InvalidArgumentException(sprintf('Build recipe baseImages contains unknown role "%s".', $role));
            }
            if (isset($seen[$role])) {
                throw new InvalidArgumentException(sprintf('Build recipe baseImages contains duplicate role "%s".', $role));
            }
            $seen[$role] = true;
            if (!is_string($reference)) {
                throw new InvalidArgumentException(sprintf('Build recipe baseImages %s must be a string.', $role));
            }
            self::assertPinnedDigest($reference, $role);
        }
        foreach (self::BASE_IMAGE_ROLES as $role) {
            if (!isset($seen[$role])) {
                throw new InvalidArgumentException(sprintf('Build recipe baseImages is missing required role "%s".', $role));
            }
        }
    }

    /** @param array<string, mixed> $value */
    public static function fromArray(array $value, string $path = 'buildRecipe'): self
    {
        $allowed = ['sha256', 'platform', 'baseImages'];
        $unknown = array_diff(array_keys($value), $allowed);
        if ($unknown !== []) {
            throw new InvalidArgumentException(sprintf('%s contains unknown key "%s".', $path, array_values($unknown)[0]));
        }
        foreach ($allowed as $key) {
            if (!array_key_exists($key, $value)) {
                throw new InvalidArgumentException(sprintf('%s is missing required key "%s".', $path, $key));
            }
        }
        if (!is_string($value['sha256']) || !is_string($value['platform']) || !is_array($value['baseImages']) || array_is_list($value['baseImages'])) {
            throw new InvalidArgumentException(sprintf('%s must define sha256, platform, and baseImages map.', $path));
        }

        return new self($value['sha256'], $value['platform'], $value['baseImages']);
    }

    /** @return array<string, mixed> */
    public function toArray(): array
    {
        $baseImages = $this->baseImages;
        ksort($baseImages, SORT_STRING);

        return [
            'sha256' => $this->sha256,
            'platform' => $this->platform,
            'baseImages' => $baseImages,
        ];
    }

    private static function assertMatches(string $value, string $pattern, string $message): void
    {
        if (preg_match($pattern, $value) !== 1) {
            throw new InvalidArgumentException($message);
        }
    }

    private static function assertPinnedDigest(string $reference, string $role): void
    {
        if ($reference !== trim($reference) || preg_match('/[\x00-\x1F\x7F\s]/', $reference) === 1) {
            throw new InvalidArgumentException(sprintf('Build recipe baseImages %s must be a registry-qualified digest reference without whitespace.', $role));
        }
        if (str_contains($reference, '://')) {
            throw new InvalidArgumentException(sprintf('Build recipe baseImages %s must not be a URL.', $role));
        }
        if (preg_match('/[|&;$()`"\'<>]/', $reference) === 1) {
            throw new InvalidArgumentException(sprintf('Build recipe baseImages %s must not contain shell metacharacters.', $role));
        }
        if (preg_match(self::PINNED_DIGEST_PATTERN, $reference) !== 1) {
            throw new InvalidArgumentException(sprintf('Build recipe baseImages %s must be a registry-qualified image reference at @sha256 with a lowercase digest.', $role));
        }
    }
}
