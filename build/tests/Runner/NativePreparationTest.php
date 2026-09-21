<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Runner;

use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;
use MageLift\Build\Protocol\PrepareRequest;
use MageLift\Build\Runner\ConfigModuleReader;
use MageLift\Build\Runner\NativePreparation;
use MageLift\Build\Runner\RequiredCapabilityProvider;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class NativePreparationTest extends TestCase
{
    public function testBuildsWorkspaceCopyAndDiscoversRuntimeDataWithoutHardcodedModules(): void
    {
        $root = sys_get_temp_dir().'/magelift-native-'.bin2hex(random_bytes(6));
        $source = $root.'/source';
        $workspace = $root.'/state';
        mkdir($source.'/app/etc', 0o700, true);
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1, 'Magento_Catalog' => 0]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        symlink($source.'/source-only.txt', $source.'/vendor/source-only-link.txt');
        file_put_contents($source.'/source-only.txt', 'unchanged');
        mkdir($source.'/.git', 0o700);
        mkdir($source.'/.magelift', 0o700);
        file_put_contents($source.'/.git/config', 'repository state');
        file_put_contents($source.'/.magelift/state.json', 'platform state');
        file_put_contents($source.'/magelift.yaml', 'project: config');
        $request = PrepareRequest::fromJson(self::request($source));
        $runner = new SuccessfulRunner();
        $preparation = new NativePreparation(
            $runner,
            $workspace,
            new FixedCapabilities(),
            new ConfigModuleReader(),
        );

        $output = $preparation->prepare($request);

        self::assertSame(PHP_VERSION, $output->phpVersion);
        self::assertContains('zend_opcache', $output->phpExtensions);
        self::assertSame(['Vendor_Custom'], $output->enabledModules);
        self::assertSame(['custom.runtime'], $output->requiredRuntimeCapabilities);
        self::assertNotEmpty($output->checksums);
        self::assertNotEmpty($runner->requests);
        self::assertSame($workspace.'/rootfs', $runner->requests[0]->workingDirectory);
        self::assertSame(getenv('HOME'), $runner->requests[0]->environment['HOME'] ?? null);
        self::assertFileDoesNotExist($workspace.'/rootfs/.git/config');
        self::assertFileDoesNotExist($workspace.'/rootfs/.magelift/state.json');
        self::assertFileDoesNotExist($workspace.'/rootfs/magelift.yaml');
        self::assertFileExists($workspace.'/rootfs/vendor/autoload.php');
        self::assertFileDoesNotExist($source.'/generated');
        self::assertSame('unchanged', file_get_contents($source.'/source-only.txt'));
    }

    public function testRejectsNonLiteralMagentoConfig(): void
    {
        $path = tempnam(sys_get_temp_dir(), 'magelift-config-');
        file_put_contents($path, '<?php return getenv("SECRET");');

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('non-literal');
        (new ConfigModuleReader())->enabledModules($path);
    }

    public function testRejectsRootComposerAuthBeforeCreatingWorkspace(): void
    {
        [$source, $workspace] = self::minimalSource();
        file_put_contents($source.'/auth.json', '{"github-oauth":{"github.com":"secret"}}');
        $runner = new SuccessfulRunner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('auth.json');
        try {
            (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
                ->prepare(PrepareRequest::fromJson(self::request($source)));
        } finally {
            self::assertDirectoryDoesNotExist($workspace.'/rootfs');
            self::assertSame([], $runner->requests);
        }
    }

    public function testRejectsMissingRequiredExtensionBeforeCreatingWorkspace(): void
    {
        [$source, $workspace] = self::minimalSource();
        $runner = new SuccessfulRunner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('missing required extensions: magelift_missing');
        try {
            (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
                ->prepare(PrepareRequest::fromJson(str_replace(
                    '"staticContent":[]',
                    '"phpExtensions":["magelift_missing"],"staticContent":[]',
                    self::request($source),
                )));
        } finally {
            self::assertDirectoryDoesNotExist($workspace.'/rootfs');
            self::assertSame([], $runner->requests);
        }
    }

    public function testChecksTheRequestedComposerVersionBeforeLifecycle(): void
    {
        [$source, $workspace] = self::minimalSource();
        mkdir($source.'/app/etc', 0o700, true);
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        $runner = new ComposerVersionRunner();
        $request = str_replace(
            '"staticContent":[]',
            '"phpExtensions":["mbstring","opcache"],"composerVersion":"2.10","staticContent":[]',
            self::request($source),
        );

        (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
            ->prepare(PrepareRequest::fromJson($request));

        self::assertSame(['composer', '--version', '--no-ansi'], $runner->requests[0]->argv);
    }

    public function testRejectsSymlinkedMagentoEnvironmentBeforeCreatingWorkspace(): void
    {
        [$source, $workspace] = self::minimalSource();
        $secret = dirname($source).'/env-secret.php';
        file_put_contents($secret, '<?php return ["db" => "secret"];');
        symlink($secret, $source.'/app/etc/env.php');
        $runner = new SuccessfulRunner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('app/etc/env.php');
        try {
            (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
                ->prepare(PrepareRequest::fromJson(self::request($source)));
        } finally {
            self::assertDirectoryDoesNotExist($workspace.'/rootfs');
            self::assertSame([], $runner->requests);
        }
    }

    public function testReportsTheFailedLifecycleStep(): void
    {
        [$source, $workspace] = self::minimalSource();
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        $runner = new FailingRunner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('build step build');
        try {
            (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
                ->prepare(PrepareRequest::fromJson(self::request($source)));
        } finally {
            self::assertCount(5, $runner->requests);
        }
    }

    public function testExecutesAValidatedHookInGraphOrder(): void
    {
        [$source, $workspace] = self::minimalSource();
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        $request = str_replace(
            '"staticContent":[]',
            '"staticContent":[],"lifecycleHooks":[{"id":"build.prepare","phase":"build","relationship":"before","target":"build","command":{"executable":"composer","arguments":["run-script","prepare"]}}]',
            self::request($source),
        );
        $runner = new SuccessfulRunner();

        (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
            ->prepare(PrepareRequest::fromJson($request));

        self::assertContains(
            ['composer', 'run-script', 'prepare'],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
    }

    public function testExecutesConfiguredStaticContentMatrix(): void
    {
        [$source, $workspace] = self::minimalSource();
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        $request = str_replace(
            '"staticContent":[]',
            '"staticContent":[{"locale":"en_US","theme":"Magento/blank"},{"locale":"fr_FR","theme":"Vendor/theme"}]',
            self::request($source),
        );
        $runner = new SuccessfulRunner();

        (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
            ->prepare(PrepareRequest::fromJson($request));

        self::assertContains(
            ['php', \MageLift\Build\Magento\DefaultStoreScaffold::COMMAND],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
        self::assertContains(
            ['bin/magento', 'setup:static-content:deploy', '--force', '--language', 'en_US', '--theme', 'Magento/blank', '--no-interaction'],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
        self::assertContains(
            ['bin/magento', 'setup:static-content:deploy', '--force', '--language', 'fr_FR', '--theme', 'Vendor/theme', '--no-interaction'],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
    }

    public function testExecutesConfiguredStaticContentStrategyAndThreads(): void
    {
        [$source, $workspace] = self::minimalSource();
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        $request = str_replace(
            '"staticContent":[]',
            '"staticContent":[{"locale":"en_US","theme":"Magento/blank","strategy":"standard","threads":2}]',
            self::request($source),
        );
        $runner = new SuccessfulRunner();

        (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
            ->prepare(PrepareRequest::fromJson($request));

        self::assertContains(
            ['bin/magento', 'setup:static-content:deploy', '--force', '--language', 'en_US', '--theme', 'Magento/blank', '-s', 'standard', '-j', '2', '--no-interaction'],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
    }

    public function testExecutesHotfixPatchesAfterComposerInstall(): void
    {
        [$source, $workspace] = self::minimalSource();
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        mkdir($source.'/m2-hotfixes', 0o700);
        file_put_contents($source.'/m2-hotfixes/b-second.patch', "diff\n");
        file_put_contents($source.'/m2-hotfixes/a-first.patch', "diff\n");
        $runner = new SuccessfulRunner();

        (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
            ->prepare(PrepareRequest::fromJson(self::request($source)));

        $argv = array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests);
        $composerInstall = ['composer', 'install', '--no-dev', '--prefer-dist', '--no-interaction', '--no-progress', '--optimize-autoloader'];
        $firstPatch = ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/a-first.patch'];
        $secondPatch = ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/b-second.patch'];
        $compile = ['bin/magento', 'setup:di:compile'];
        self::assertContains($composerInstall, $argv);
        self::assertContains($firstPatch, $argv);
        self::assertContains($secondPatch, $argv);
        self::assertContains($compile, $argv);
        self::assertLessThan(
            array_search($firstPatch, $argv, true),
            array_search($composerInstall, $argv, true),
        );
        self::assertLessThan(
            array_search($secondPatch, $argv, true),
            array_search($firstPatch, $argv, true),
        );
        self::assertLessThan(
            array_search($compile, $argv, true),
            array_search($secondPatch, $argv, true),
        );
    }

    public function testStopsBeforeCompileWhenQualityPatchToolRejectsAnId(): void
    {
        [$source, $workspace] = self::minimalSource();
        mkdir($source.'/vendor', 0o700, true);
        file_put_contents($source.'/app/etc/config.php', "<?php\nreturn ['modules' => ['Vendor_Custom' => 1]];\n");
        file_put_contents($source.'/vendor/autoload.php', '<?php return true;');
        file_put_contents($source.'/composer.lock', json_encode([
            'packages' => [['name' => 'magento/quality-patches']],
        ], JSON_THROW_ON_ERROR));
        file_put_contents($source.'/.magento.env.yaml', "stage:\n  build:\n    QUALITY_PATCHES:\n      - ACSD-999\n");
        $runner = new QualityPatchFailureRunner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('ACSD-123 is unavailable');
        try {
            $json = str_replace('"staticContent":[]', '"staticContent":[],"qualityPatches":["ACSD-123"]', self::request($source));
            (new NativePreparation($runner, $workspace, new FixedCapabilities(), new ConfigModuleReader()))
                ->prepare(PrepareRequest::fromJson($json));
        } finally {
            $argv = array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests);
            self::assertContains(['php', 'vendor/bin/magento-patches', 'apply', 'ACSD-123'], $argv);
            self::assertNotContains(['bin/magento', 'setup:di:compile'], $argv);
        }
    }

    /** @return array{string, string} */
    private static function minimalSource(): array
    {
        $root = sys_get_temp_dir().'/magelift-sensitive-'.bin2hex(random_bytes(6));
        $source = $root.'/source';
        mkdir($source.'/app/etc', 0o700, true);
        file_put_contents($source.'/source-only.txt', 'unchanged');

        return [$source, $root.'/state'];
    }

    private static function request(string $root): string
    {
        [$major, $minor] = explode('.', PHP_VERSION);
        return '{"protocolVersion":1,"stage":"prepare","prepare":{"repositoryRoot":"'.$root.'","sourceRevision":"'.str_repeat('a', 40).'","application":{"edition":"open-source","version":"2.4.9","mode":"integrated","webRuntime":"nginx-fpm"},"phpVersion":"'.$major.'.'.$minor.'","composerVersion":"2.10","compatibilityStatus":"supported","inputFiles":[{"path":"source-only.txt","sha256":"'.hash('sha256', 'unchanged').'"}],"staticContent":[]}}';
    }
}

final class SuccessfulRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;
        if ($request->argv === ['composer', '--version', '--no-ansi']) {
            return new ProcessResult(0, "Composer version 2.10.2 2026-07-01 11:24:45\n", '');
        }
        self::materializeComposerOutput($request);

        return new ProcessResult(0, '', '');
    }

    public static function materializeComposerOutput(ProcessRequest $request): void
    {
        if ($request->argv === ['composer', '--version', '--no-ansi']) {
            return;
        }
        if ($request->argv[0] !== 'composer' || $request->argv[1] !== 'install') {
            return;
        }
        mkdir($request->workingDirectory.'/vendor', 0o700, true);
        file_put_contents($request->workingDirectory.'/vendor/autoload.php', '<?php return true;');
    }
}

final class FailingRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;
        if ($request->argv === ['composer', '--version', '--no-ansi']) {
            return new ProcessResult(0, "Composer version 2.10.2 2026-07-01 11:24:45\n", '');
        }

        return new ProcessResult(count($this->requests) === 5 ? 17 : 0, '', 'failed');
    }
}

final class ComposerVersionRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;
        if ($request->argv === ['composer', '--version', '--no-ansi']) {
            return new ProcessResult(0, "Composer version 2.10.2 2026-07-01 11:24:45\n", '');
        }

        SuccessfulRunner::materializeComposerOutput($request);

        return new ProcessResult(0, '', '');
    }
}

final class QualityPatchFailureRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;
        if ($request->argv === ['composer', '--version', '--no-ansi']) {
            return new ProcessResult(0, "Composer version 2.10.2 2026-07-01 11:24:45\n", '');
        }
        SuccessfulRunner::materializeComposerOutput($request);
        if ($request->argv === ['php', 'vendor/bin/magento-patches', 'apply', 'ACSD-123']) {
            return new ProcessResult(17, '', 'ACSD-123 is unavailable');
        }

        return new ProcessResult(0, '', '');
    }
}

final class FixedCapabilities implements RequiredCapabilityProvider
{
    public function capabilitiesFor(PrepareRequest $request): array
    {
        return ['custom.runtime'];
    }
}
