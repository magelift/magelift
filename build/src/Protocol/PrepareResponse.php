<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

use InvalidArgumentException;

final readonly class PrepareResponse
{
    /**
     * @param list<string> $phpExtensions
     * @param list<string> $enabledModules
     * @param list<array{path: string, sha256: string}> $checksums
     * @param list<string> $requiredRuntimeCapabilities
     */
    public function __construct(
        public string $preparedArtifact,
        public string $phpVersion,
        public array $phpExtensions,
        public array $enabledModules,
        public array $checksums,
        public array $requiredRuntimeCapabilities,
    ) {
        ProtocolJson::relativePath($this->preparedArtifact, 'prepared artifact');
        ProtocolJson::nonEmptyString($this->phpVersion, 'prepared PHP version');
        self::validateUniqueStrings($this->phpExtensions, 'PHP extensions');
        self::validateUniqueStrings($this->enabledModules, 'enabled modules');
        self::validateUniqueStrings($this->requiredRuntimeCapabilities, 'runtime capabilities', true);
        if ($this->checksums === []) {
            throw new InvalidArgumentException('Prepared artifact checksums are required.');
        }
        $paths = [];
        foreach ($this->checksums as $checksum) {
            if (
                !is_array($checksum)
                || array_diff(array_keys($checksum), ['path', 'sha256']) !== []
                || array_diff(['path', 'sha256'], array_keys($checksum)) !== []
            ) {
                throw new InvalidArgumentException('Artifact checksums must contain only path and sha256.');
            }
            $path = ProtocolJson::relativePath($checksum['path'], 'artifact checksum');
            ProtocolJson::checksum($checksum['sha256'], 'artifact');
            if (isset($paths[$path])) {
                throw new InvalidArgumentException('Duplicate artifact checksum path.');
            }
            $paths[$path] = true;
        }
        ProtocolJson::assertNoSecrets([
            'preparedArtifact' => $this->preparedArtifact,
            'phpVersion' => $this->phpVersion,
            'phpExtensions' => $this->phpExtensions,
            'enabledModules' => $this->enabledModules,
            'checksums' => $this->checksums,
            'requiredRuntimeCapabilities' => $this->requiredRuntimeCapabilities,
        ]);
    }

    public function toCanonicalJson(): string
    {
        $phpExtensions = $this->phpExtensions;
        $enabledModules = $this->enabledModules;
        $checksums = array_map(
            static fn (array $checksum): array => ['path' => $checksum['path'], 'sha256' => $checksum['sha256']],
            $this->checksums,
        );
        $capabilities = $this->requiredRuntimeCapabilities;
        sort($phpExtensions, SORT_STRING);
        sort($enabledModules, SORT_STRING);
        usort($checksums, static fn (array $a, array $b): int => strcmp($a['path'], $b['path']));
        sort($capabilities, SORT_STRING);

        return ProtocolJson::encode([
            'protocolVersion' => ProtocolJson::VERSION,
            'stage' => ProtocolOperation::Prepare->value,
            'prepare' => [
                'preparedArtifact' => $this->preparedArtifact,
                'phpVersion' => $this->phpVersion,
                'phpExtensions' => $phpExtensions,
                'enabledModules' => $enabledModules,
                'checksums' => $checksums,
                'requiredRuntimeCapabilities' => $capabilities,
            ],
        ]);
    }

    /** @param list<string> $values */
    private static function validateUniqueStrings(array $values, string $name, bool $stableIds = false): void
    {
        if ($values === [] || !array_is_list($values)) {
            throw new InvalidArgumentException(sprintf('%s cannot be empty.', ucfirst($name)));
        }
        foreach ($values as $value) {
            if (!is_string($value) || preg_match('/^\s*$/u', $value) === 1 || ($stableIds && preg_match('/^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$/D', $value) !== 1)) {
                throw new InvalidArgumentException(sprintf('Invalid %s value.', $name));
            }
        }
        if (count($values) !== count(array_unique($values))) {
            throw new InvalidArgumentException(sprintf('Duplicate %s value.', $name));
        }
    }
}
