<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Magento\InitialDatabaseBootstrap;
use MageLift\Build\Process\NativeProcessRunner;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class InitialDatabaseBootstrapTest extends TestCase
{
    public function testLifecycleInlineSourceIsSelfContained(): void
    {
        $source = InitialDatabaseBootstrap::inlineRuntimeSource();

        self::assertStringNotContainsString('/opt/magelift', $source);
        self::assertStringNotContainsString('vendor/autoload', $source);
        self::assertStringContainsString('SHOW TABLES', $source);
        self::assertStringContainsString('setup:db-schema:upgrade', $source);
        self::assertStringContainsString('magelift_ensure_initial_database', $source);
    }

    public function testRuntimeSubprocessPreservesExitAndBothStreams(): void
    {
        $runtime = realpath(__DIR__.'/../../bin/ensure-initial-database.runtime.php');
        self::assertIsString($runtime);
        $child = [PHP_BINARY, '-r', 'fwrite(STDERR, str_repeat("e", 131072)); fwrite(STDOUT, str_repeat("o", 131072)); exit(7);'];
        $source = 'require '.var_export($runtime, true).'; exit(magelift_initial_db_run_process('
            .var_export($child, true).', getcwd()));';
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', $source], getcwd() ?: '.', timeoutSeconds: 10.0,
        ));
        self::assertSame(7, $result->exitCode);
        self::assertSame(str_repeat('o', 131072), $result->stdout);
        self::assertSame(str_repeat('e', 131072), $result->stderr);
    }

    public function testInlineRuntimeFailsClosedWithoutBuildPackage(): void
    {
        $root = $this->magentoRoot(['db' => ['connection' => ['default' => []]]]);
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', InitialDatabaseBootstrap::inlineRuntimeSource()], $root, timeoutSeconds: 10.0,
        ));
        self::assertSame(1, $result->exitCode);
        self::assertStringNotContainsString('Stack trace', $result->stderr);
        self::assertStringContainsString('Database connection host is not configured.', $result->stderr);
    }

    public function testInlineRuntimeDoesNotExposeUnexpectedConfigurationErrors(): void
    {
        $root = $this->magentoRoot([]);
        file_put_contents($root.'/app/etc/env.php', '<?php throw new RuntimeException("synthetic-sensitive-value");');
        $result = (new NativeProcessRunner())->run(new ProcessRequest(
            [PHP_BINARY, '-r', InitialDatabaseBootstrap::inlineRuntimeSource()], $root, timeoutSeconds: 10.0,
        ));
        self::assertSame(1, $result->exitCode);
        self::assertStringNotContainsString('synthetic-sensitive-value', $result->stderr);
        self::assertStringNotContainsString('Stack trace', $result->stderr);
        self::assertStringContainsString('Initial database bootstrap failed.', $result->stderr);
    }

    protected function tearDown(): void
    {
        putenv('MAGENTO_DC__OVERRIDE');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD');
    }

    public function testResolvesDatabaseConnectionFromEnvPlaceholders(): void
    {
        $root = $this->magentoRoot([
            'db' => [
                'connection' => [
                    'default' => [
                        'host' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST)',
                        'dbname' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME)',
                        'username' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME)',
                        'password' => '#env(MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD)',
                    ],
                ],
            ],
        ]);
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST=db.internal');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME=magento');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME=app');
        putenv('MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD=secret');

        $connection = (new InitialDatabaseBootstrap())->resolveDatabaseConnection($root);

        self::assertSame([
            'host' => 'db.internal',
            'dbname' => 'magento',
            'username' => 'app',
            'password' => 'secret',
        ], $connection);
    }

    public function testMergesMagentoDeploymentConfigOverride(): void
    {
        $root = $this->magentoRoot([
            'db' => [
                'connection' => [
                    'default' => [
                        'host' => 'stale',
                        'dbname' => 'stale',
                        'username' => 'stale',
                        'password' => 'stale',
                    ],
                ],
            ],
        ]);
        putenv('MAGENTO_DC__OVERRIDE='.json_encode([
            'db' => [
                'connection' => [
                    'default' => [
                        'host' => 'live',
                        'dbname' => 'shop',
                        'username' => 'user',
                        'password' => 'pass',
                    ],
                ],
            ],
        ], JSON_UNESCAPED_SLASHES));

        $connection = (new InitialDatabaseBootstrap())->resolveDatabaseConnection($root);

        self::assertSame('live', $connection['host']);
        self::assertSame('shop', $connection['dbname']);
    }

    public function testRejectsMissingEnvPhp(): void
    {
        $root = sys_get_temp_dir().'/magelift-bootstrap-'.bin2hex(random_bytes(4));
        mkdir($root, 0o755, true);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('app/etc/env.php is missing');

        (new InitialDatabaseBootstrap())->resolveDatabaseConnection($root);
    }

    public function testRejectsInvalidDeploymentOverride(): void
    {
        $root = $this->magentoRoot(['db' => ['connection' => ['default' => ['host' => 'h', 'dbname' => 'd', 'username' => 'u', 'password' => 'p']]]]);
        putenv('MAGENTO_DC__OVERRIDE=not-json');

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('MAGENTO_DC__OVERRIDE');

        (new InitialDatabaseBootstrap())->resolveDatabaseConnection($root);
    }

    public function testSkipsPopulatedDatabaseWithoutRunningMagento(): void
    {
        $root = $this->magentoRootWithMagento([
            'db' => ['connection' => ['default' => ['host' => 'h', 'dbname' => 'd', 'username' => 'u', 'password' => 'p']]],
        ]);
        $runner = new BootstrapProcessRecorder();

        ob_start();
        (new InitialDatabaseBootstrap($runner, static fn (): int => 3))->ensure($root);
        $output = (string) ob_get_clean();

        self::assertSame([], $runner->requests);
        self::assertSame(InitialDatabaseBootstrap::SKIP_PREFIX.' table_count=3'."\n", $output);
    }

    public function testRunsSchemaAndDataUpgradesOnEmptyDatabase(): void
    {
        $root = $this->magentoRootWithMagento([
            'db' => ['connection' => ['default' => ['host' => 'h', 'dbname' => 'd', 'username' => 'u', 'password' => 'p']]],
        ]);
        $runner = new BootstrapProcessRecorder();

        (new InitialDatabaseBootstrap($runner, static fn (): int => 0))->ensure($root);

        self::assertSame([
            [$root.'/bin/magento', 'setup:db-schema:upgrade', '--no-interaction'],
            [$root.'/bin/magento', 'setup:db-data:upgrade', '--no-interaction'],
        ], array_map(static fn (ProcessRequest $request): array => $request->argv, $runner->requests));
    }

    public function testPropagatesFailingSchemaUpgrade(): void
    {
        $root = $this->magentoRootWithMagento([
            'db' => ['connection' => ['default' => ['host' => 'h', 'dbname' => 'd', 'username' => 'u', 'password' => 'p']]],
        ]);
        $runner = new BootstrapProcessRecorder([1]);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('setup:db-schema:upgrade failed');

        (new InitialDatabaseBootstrap($runner, static fn (): int => 0))->ensure($root);
    }

    /** @param array<mixed, mixed> $env */
    private function magentoRoot(array $env): string
    {
        $root = sys_get_temp_dir().'/magelift-bootstrap-'.bin2hex(random_bytes(4));
        mkdir($root.'/app/etc', 0o755, true);
        file_put_contents(
            $root.'/app/etc/env.php',
            "<?php\nreturn ".var_export($env, true).";\n",
        );

        return $root;
    }

    /** @param array<mixed, mixed> $env */
    private function magentoRootWithMagento(array $env): string
    {
        $root = $this->magentoRoot($env);
        mkdir($root.'/bin', 0o755, true);
        file_put_contents($root.'/bin/magento', "#!/bin/sh\nexit 0\n");

        return $root;
    }
}

final class BootstrapProcessRecorder implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    /** @param list<int> $exitCodes */
    public function __construct(private array $exitCodes = [])
    {
    }

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;
        $exitCode = $this->exitCodes[$this->countRequests() - 1] ?? 0;

        return new ProcessResult($exitCode, '', '');
    }

    private function countRequests(): int
    {
        return count($this->requests);
    }
}
