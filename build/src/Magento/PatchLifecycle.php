<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use RuntimeException;

/**
 * Selects the patch commands for one immutable project source tree.
 */
final class PatchLifecycle
{
    /** @param list<CommandInterface> $commands */
    public static function hasHostPatchFallback(array $commands): bool
    {
        foreach ($commands as $command) {
            if ($command instanceof PatchCommand) {
                return true;
            }
        }

        return false;
    }

    /**
     * @param list<string> $hotfixPatches
     * @param list<string> $qualityPatchIds
     * @return list<CommandInterface>
     */
    public static function commands(string $projectRoot, array $hotfixPatches, array $qualityPatchIds = []): array
    {
        $qualityPatchIds = QualityPatchConfig::normalize($qualityPatchIds);
        $packages = ComposerPackageInventory::read($projectRoot);
        $hasQualityPatches = in_array('magento/quality-patches', $packages, true);

        if ($qualityPatchIds !== [] && !$hasQualityPatches) {
            throw new RuntimeException(
                'qualityPatches is configured, but the production Composer lock does not provide '
                .'magento/quality-patches. Add the supported package and lock it before building.',
            );
        }

        $commands = [];
        if ($qualityPatchIds !== []) {
            $commands[] = new Command(Executable::Php, [
                'vendor/bin/magento-patches',
                'apply',
                ...$qualityPatchIds,
            ]);
        }
        if ($hotfixPatches !== []) {
            usort($hotfixPatches, static fn (string $left, string $right): int => [basename($left), $left] <=> [basename($right), $right]);
            array_push($commands, ...PatchApplier::commands($hotfixPatches));
        }

        return $commands;
    }
}
