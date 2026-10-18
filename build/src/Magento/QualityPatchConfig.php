<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use RuntimeException;

/**
 * Validates Quality Patch IDs from magelift.yaml / the prepare protocol.
 * Build never reads leftover .magento.env.yaml as a live control plane.
 */
final class QualityPatchConfig
{
    /**
     * @param list<mixed> $ids
     * @return list<string>
     */
    public static function normalize(array $ids): array
    {
        $normalized = [];
        $seen = [];
        foreach ($ids as $index => $value) {
            if (!is_string($value)) {
                throw new RuntimeException(sprintf('qualityPatches[%d] must be a string Quality Patch ID.', $index));
            }
            $id = self::parseIdentifier($value);
            if (isset($seen[$id])) {
                throw new RuntimeException(sprintf('Duplicate qualityPatches ID "%s".', $id));
            }
            $seen[$id] = true;
            $normalized[] = $id;
        }

        return $normalized;
    }

    public static function parseIdentifier(string $value): string
    {
        $value = trim($value);
        if (
            strlen($value) >= 2
            && (($value[0] === '"' && $value[strlen($value) - 1] === '"')
                || ($value[0] === "'" && $value[strlen($value) - 1] === "'"))
        ) {
            $value = substr($value, 1, -1);
        }
        if (preg_match('/^[A-Za-z0-9][A-Za-z0-9._-]*$/D', $value) !== 1) {
            throw new RuntimeException(sprintf('invalid qualityPatches ID "%s"', $value));
        }

        return $value;
    }
}
