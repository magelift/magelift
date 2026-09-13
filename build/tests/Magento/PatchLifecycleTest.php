<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Magento\PatchLifecycle;
use MageLift\Build\Magento\QualityPatchConfig;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class PatchLifecycleTest extends TestCase
{
    public function testMagentoYamlIdsWinOverLeftoverEnvYaml(): void
    {
        $root = $this->tempProject();
        $this->writeLock($root, ['magento/quality-patches']);
        file_put_contents($root.'/.magento.env.yaml', "stage:\n  build:\n    QUALITY_PATCHES:\n      - ACSD-999\n");

        $commands = PatchLifecycle::commands($root, [
            'm2-hotfixes/a-first.patch',
            'm2-hotfixes/b-second.patch',
        ], ['ACSD-123']);

        self::assertSame([
            ['php', 'vendor/bin/magento-patches', 'apply', 'ACSD-123'],
            ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/a-first.patch'],
            ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/b-second.patch'],
        ], array_map(static fn ($command): array => $command->argv(), $commands));
        self::assertTrue(PatchLifecycle::hasHostPatchFallback($commands));
    }

    public function testStandaloneQualityPatchesRunBeforeAlphabeticalLocalFallback(): void
    {
        $root = $this->tempProject();
        $this->writeLock($root, ['magento/quality-patches']);

        $commands = PatchLifecycle::commands($root, [
            'm2-hotfixes/b-second.patch',
            'm2-hotfixes/a-first.patch',
        ], ['MAGETWO-67097', 'ACSD-456']);

        self::assertSame([
            ['php', 'vendor/bin/magento-patches', 'apply', 'MAGETWO-67097', 'ACSD-456'],
            ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/a-first.patch'],
            ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/b-second.patch'],
        ], array_map(static fn ($command): array => $command->argv(), $commands));
        self::assertTrue(PatchLifecycle::hasHostPatchFallback($commands));
    }

    public function testQualityPatchIdsWithoutAnApplyingPackageFailBeforeBuild(): void
    {
        $root = $this->tempProject();
        $this->writeLock($root, [], ['magento/quality-patches']);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('does not provide magento/quality-patches');
        PatchLifecycle::commands($root, [], ['ACSD-123']);
    }

    public function testDuplicateQualityPatchIdsAreRejected(): void
    {
        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('Duplicate qualityPatches ID');
        QualityPatchConfig::normalize(['ACSD-123', 'ACSD-123']);
    }

    public function testQualityPatchIdsInDevOnlyPackagesDoNotEnableProductionPatching(): void
    {
        $root = $this->tempProject();
        $this->writeLock($root, [], ['magento/quality-patches']);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('does not provide magento/quality-patches');
        PatchLifecycle::commands($root, [], ['ACSD-123']);
    }

    public function testMissingPatchInputsAreAStableNoOp(): void
    {
        $root = $this->tempProject();

        self::assertSame([], PatchLifecycle::commands($root, []));
    }

    public function testCloudLifecycleDoesNotReadEnvYamlWhenIdsComeFromMagelift(): void
    {
        $root = $this->tempProject();
        $this->writeLock($root, ['magento/magento-cloud-patches', 'magento/quality-patches']);
        file_put_contents($root.'/.magento.env.yaml', "stage:\n  build:\n    QUALITY_PATCHES:\n      - ACSD-999\n");

        $commands = PatchLifecycle::commands($root, ['m2-hotfixes/a-first.patch'], ['ACSD-123']);

        self::assertSame([
            ['php', 'vendor/bin/magento-patches', 'apply', 'ACSD-123'],
            ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/a-first.patch'],
        ], array_map(static fn ($command): array => $command->argv(), $commands));
    }

    private function writeLock(string $root, array $packages, array $devPackages = []): void
    {
        file_put_contents($root.'/composer.lock', json_encode([
            'packages' => array_map(static fn (string $name): array => ['name' => $name], $packages),
            'packages-dev' => array_map(static fn (string $name): array => ['name' => $name], $devPackages),
        ], JSON_THROW_ON_ERROR));
    }

    private function tempProject(): string
    {
        $root = sys_get_temp_dir().'/magelift-patch-lifecycle-'.bin2hex(random_bytes(6));
        mkdir($root, 0o700, true);

        return $root;
    }
}
