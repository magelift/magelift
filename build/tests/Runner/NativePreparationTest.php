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
            self::assertCount(3, $runner->requests);
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

        self::assertSame(['composer', 'run-script', 'prepare'], $runner->requests[2]->argv);
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
            ['bin/magento', 'setup:static-content:deploy', '--language', 'en_US', '--theme', 'Magento/blank', '--no-interaction'],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
        self::assertContains(
            ['bin/magento', 'setup:static-content:deploy', '--language', 'fr_FR', '--theme', 'Vendor/theme', '--no-interaction'],
            array_map(static fn (ProcessRequest $process): array => $process->argv, $runner->requests),
        );
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
        return '{"protocolVersion":1,"stage":"prepare","prepare":{"repositoryRoot":"'.$root.'","sourceRevision":"'.str_repeat('a', 40).'","application":{"edition":"open-source","version":"2.4.9","mode":"integrated","webRuntime":"nginx-fpm"},"phpVersion":"'.$major.'.'.$minor.'","compatibilityStatus":"supported","inputFiles":[{"path":"source-only.txt","sha256":"'.hash('sha256', 'unchanged').'"}],"staticContent":[]}}';
    }
}

final class SuccessfulRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;

        return new ProcessResult(0, '', '');
    }
}

final class FailingRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;

        return new ProcessResult(count($this->requests) === 3 ? 17 : 0, '', 'failed');
    }
}

final class FixedCapabilities implements RequiredCapabilityProvider
{
    public function capabilitiesFor(PrepareRequest $request): array
    {
        return ['custom.runtime'];
    }
}
