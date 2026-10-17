<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Magento\CloudPatchCommand;
use MageLift\Build\Process\NativeProcessRunner;
use MageLift\Build\Process\ProcessRequest;
use PHPUnit\Framework\TestCase;

final class CloudPatchCommandTest extends TestCase
{
    private string|false $previousProcessEnv;
    private bool $hadEnvironmentValue;
    private mixed $previousEnvironmentValue;

    protected function setUp(): void
    {
        $this->previousProcessEnv = getenv('QUALITY_PATCHES');
        $this->hadEnvironmentValue = array_key_exists('QUALITY_PATCHES', $_ENV);
        $this->previousEnvironmentValue = $_ENV['QUALITY_PATCHES'] ?? null;
    }

    protected function tearDown(): void
    {
        putenv($this->previousProcessEnv === false ? 'QUALITY_PATCHES' : 'QUALITY_PATCHES='.$this->previousProcessEnv);
        if ($this->hadEnvironmentValue) {
            $_ENV['QUALITY_PATCHES'] = $this->previousEnvironmentValue;
        } else {
            unset($_ENV['QUALITY_PATCHES']);
        }
    }

    public function testBootstrapSetsQualityPatchesEnvAndInvokesEcePatchesArgv(): void
    {
        $root = $this->tempProject();
        $this->installFakeEcePatches($root);
        putenv('QUALITY_PATCHES=ACSD-ambient');
        $_ENV['QUALITY_PATCHES'] = 'ACSD-ambient';

        $command = new CloudPatchCommand(['ACSD-123', 'MAGETWO-67097']);
        $runner = new NativeProcessRunner();
        $result = $command->run($runner, new ProcessRequest(
            $command->argv(),
            $root,
            [],
            30,
            null,
        ));

        self::assertTrue($result->succeeded(), $result->stderr);
        $capture = json_decode((string) file_get_contents($root.'/.ece-capture.json'), true, 512, JSON_THROW_ON_ERROR);
        self::assertSame(['ACSD-123', 'MAGETWO-67097'], $capture['qualityPatches']);
        self::assertSame([$root.'/vendor/bin/ece-patches', 'apply'], $capture['argv']);
        self::assertSame(2, $capture['argc']);
    }

    public function testExplicitEmptyQualityIdsOverrideAmbientConfig(): void
    {
        $root = $this->tempProject();
        $this->installFakeEcePatches($root);
        file_put_contents($root.'/.magento.env.yaml', "stage:\n  build:\n    QUALITY_PATCHES:\n      - ACSD-999\n");
        putenv('QUALITY_PATCHES=ACSD-ambient');
        $_ENV['QUALITY_PATCHES'] = 'ACSD-ambient';

        $command = new CloudPatchCommand([]);
        $runner = new NativeProcessRunner();
        $result = $command->run($runner, new ProcessRequest(
            $command->argv(),
            $root,
            [],
            30,
            null,
        ));

        self::assertTrue($result->succeeded(), $result->stderr);
        $capture = json_decode((string) file_get_contents($root.'/.ece-capture.json'), true, 512, JSON_THROW_ON_ERROR);
        self::assertSame([], $capture['qualityPatches']);
    }

    private function installFakeEcePatches(string $root): void
    {
        mkdir($root.'/vendor/bin', 0o700, true);
        $script = <<<'PHP'
<?php
if (($_SERVER['argv'][1] ?? null) !== 'apply' || ($_SERVER['argc'] ?? 0) !== 2) {
    fwrite(STDERR, "Expected the apply subcommand.\n");
    exit(17);
}
file_put_contents(getcwd().'/.ece-capture.json', json_encode([
    'qualityPatches' => $_ENV['QUALITY_PATCHES'] ?? null,
    'argv' => $_SERVER['argv'] ?? null,
    'argc' => $_SERVER['argc'] ?? null,
], JSON_THROW_ON_ERROR));
PHP;
        file_put_contents($root.'/vendor/bin/ece-patches', $script);
    }

    private function tempProject(): string
    {
        $root = sys_get_temp_dir().'/magelift-cloud-patch-'.bin2hex(random_bytes(6));
        mkdir($root, 0o700, true);

        return $root;
    }
}
