<?php

declare(strict_types=1);

namespace MageLift\Build\Artifact;

use InvalidArgumentException;
use JsonException;

final readonly class ArtifactManifest
{
    private const DIGEST_PATTERN = '/^sha256:[a-f0-9]{64}$/D';
    private const CHECKSUM_PATTERN = '/^[a-f0-9]{64}$/D';
    private const REVISION_PATTERN = '/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/D';
    private const VERSION_PATTERN = '/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/D';
    private const CAPABILITY_PATTERN = '/^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$/D';
    private const PHP_EXTENSION_PATTERN = '/^[a-z][a-z0-9_-]*$/D';
    private const MAGENTO_MODULE_PATTERN = '/^[A-Z][A-Za-z0-9]*_[A-Z][A-Za-z0-9]*$/D';

    /**
     * @param list<string> $phpExtensions
     * @param list<string> $enabledModules
     * @param array<string, string> $buildInputs
     * @param array<string, string> $checksums Paths mapped to lowercase SHA-256 checksums.
     * @param array<string, list<string>> $staticContentMatrix Locales mapped to themes.
     * @param list<string> $requiredRuntimeCapabilities
     */
    public function __construct(
        public string $sourceRevision,
        public string $imageDigest,
        public string $mageLiftVersion,
        public string $buildPackageVersion,
        public MagentoEdition $magentoEdition,
        public string $magentoVersion,
        public string $phpVersion,
        public array $phpExtensions,
        public array $enabledModules,
        public array $buildInputs,
        public array $checksums,
        public array $staticContentMatrix,
        public array $requiredRuntimeCapabilities,
        public CompatibilityStatus $compatibilityStatus,
    ) {
        self::assertMatches($this->sourceRevision, self::REVISION_PATTERN, 'Source revision must be a lowercase 40- or 64-character commit hash.');
        self::assertMatches($this->imageDigest, self::DIGEST_PATTERN, 'Image digest must be a lowercase SHA-256 OCI digest.');
        self::assertMatches($this->mageLiftVersion, self::VERSION_PATTERN, 'MageLift version must be a semantic version.');
        self::assertMatches($this->buildPackageVersion, self::VERSION_PATTERN, 'Build package version must be a semantic version.');
        self::assertMatches($this->magentoVersion, self::VERSION_PATTERN, 'Magento version must be a semantic version.');
        self::assertMatches($this->phpVersion, self::VERSION_PATTERN, 'PHP version must be a semantic version.');

        self::assertUniqueList($this->phpExtensions, 'PHP extensions');
        foreach ($this->phpExtensions as $extension) {
            self::assertMatches($extension, self::PHP_EXTENSION_PATTERN, sprintf('Invalid PHP extension name "%s".', $extension));
        }

        self::assertUniqueList($this->enabledModules, 'Enabled modules');
        foreach ($this->enabledModules as $module) {
            self::assertMatches($module, self::MAGENTO_MODULE_PATTERN, sprintf('Invalid Magento module name "%s".', $module));
        }

        self::assertStringMap($this->buildInputs, 'Build inputs');
        if ($this->checksums === []) {
            throw new InvalidArgumentException('Artifact checksums cannot be empty.');
        }
        self::assertStringMap($this->checksums, 'Artifact checksums');
        foreach ($this->checksums as $path => $checksum) {
            if (str_starts_with($path, '/') || in_array('..', explode('/', $path), true)) {
                throw new InvalidArgumentException(sprintf('Artifact checksum path "%s" must be relative and cannot contain parent traversal.', $path));
            }
            self::assertMatches($checksum, self::CHECKSUM_PATTERN, sprintf('Invalid SHA-256 checksum for "%s".', $path));
        }

        foreach ($this->staticContentMatrix as $locale => $themes) {
            if (!is_string($locale) || $locale === '' || !is_array($themes) || !array_is_list($themes) || $themes === []) {
                throw new InvalidArgumentException('Each static-content locale must have a non-empty theme list.');
            }
            self::assertUniqueList($themes, sprintf('Static-content themes for %s', $locale));
        }

        if ($this->requiredRuntimeCapabilities === []) {
            throw new InvalidArgumentException('Required runtime capabilities cannot be empty.');
        }
        self::assertUniqueList($this->requiredRuntimeCapabilities, 'Required runtime capabilities');
        foreach ($this->requiredRuntimeCapabilities as $capability) {
            self::assertMatches($capability, self::CAPABILITY_PATTERN, sprintf('Invalid runtime capability "%s".', $capability));
        }
    }

    /** @throws JsonException */
    public function toCanonicalJson(): string
    {
        return json_encode(
            self::sortKeysRecursively($this->toArray()),
            JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES,
        );
    }

    /** @return array<string, mixed> */
    public function toArray(): array
    {
        $staticContentMatrix = [];
        foreach ($this->staticContentMatrix as $locale => $themes) {
            $staticContentMatrix[$locale] = self::sortedList($themes);
        }

        return [
            'sourceRevision' => $this->sourceRevision,
            'imageDigest' => $this->imageDigest,
            'mageLiftVersion' => $this->mageLiftVersion,
            'buildPackageVersion' => $this->buildPackageVersion,
            'magento' => ['edition' => $this->magentoEdition->value, 'version' => $this->magentoVersion],
            'php' => ['version' => $this->phpVersion, 'extensions' => self::sortedList($this->phpExtensions)],
            'enabledModules' => self::sortedList($this->enabledModules),
            'buildInputs' => $this->buildInputs,
            'checksums' => $this->checksums,
            'staticContentMatrix' => $staticContentMatrix,
            'requiredRuntimeCapabilities' => self::sortedList($this->requiredRuntimeCapabilities),
            'compatibilityStatus' => $this->compatibilityStatus->value,
        ];
    }

    private static function assertMatches(string $value, string $pattern, string $message): void
    {
        if (preg_match($pattern, $value) !== 1) {
            throw new InvalidArgumentException($message);
        }
    }

    /** @param list<string> $values */
    private static function assertUniqueList(array $values, string $name): void
    {
        if (!array_is_list($values)) {
            throw new InvalidArgumentException(sprintf('%s must be a list.', $name));
        }
        foreach ($values as $value) {
            if (!is_string($value) || $value === '') {
                throw new InvalidArgumentException(sprintf('%s must contain non-empty strings.', $name));
            }
        }
        if (count($values) !== count(array_unique($values))) {
            throw new InvalidArgumentException(sprintf('%s cannot contain duplicates.', $name));
        }
    }

    /** @param array<array-key, mixed> $values */
    private static function assertStringMap(array $values, string $name): void
    {
        if (array_is_list($values) && $values !== []) {
            throw new InvalidArgumentException(sprintf('%s must be a map.', $name));
        }
        foreach ($values as $key => $value) {
            if (!is_string($key) || $key === '' || !is_string($value) || $value === '') {
                throw new InvalidArgumentException(sprintf('%s must map non-empty strings to non-empty strings.', $name));
            }
        }
    }

    private static function sortKeysRecursively(mixed $value): mixed
    {
        if (!is_array($value)) {
            return $value;
        }
        if (array_is_list($value)) {
            return array_map(self::sortKeysRecursively(...), $value);
        }
        ksort($value, SORT_STRING);
        foreach ($value as &$entry) {
            $entry = self::sortKeysRecursively($entry);
        }
        unset($entry);

        return $value;
    }

    /**
     * @param list<string> $values
     * @return list<string>
     */
    private static function sortedList(array $values): array
    {
        sort($values, SORT_STRING);

        return $values;
    }
}
