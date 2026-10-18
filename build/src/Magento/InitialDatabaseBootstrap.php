<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use MageLift\Build\Process\NativeProcessRunner;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessRunner;
use RuntimeException;

/**
 * On a database with zero tables, setup:upgrade can fail before schema exists
 * because Magento reads search configuration that assumes store scope. Official
 * schema and data upgrades on an empty database avoid that path. Populated
 * databases are left untouched.
 *
 * Runtime deploy uses {@see lifecyclePhpArguments()}: the helper source is read
 * from build/bin/ensure-initial-database.runtime.php and passed to PHP as inline code.
 * The magelift/build Composer package is not installed in the application image.
 */
final class InitialDatabaseBootstrap
{
    public const SKIP_PREFIX = 'SKIP_SCHEMA_DATA_BOOTSTRAP';

    private const RUNTIME_SCRIPT = __DIR__.'/../../bin/ensure-initial-database.runtime.php';

    /**
     * @param null|callable(array{host: string, dbname: string, username: string, password: string}): int $tableCounter
     */
    public function __construct(
        private ?ProcessRunner $runner = null,
        private $tableCounter = null,
    ) {
    }

    /** @return list<string> */
    public static function lifecyclePhpArguments(): array
    {
        return ['-r', self::inlineRuntimeSource()];
    }

    public function ensure(string $magentoRoot): void
    {
        self::loadRuntimeLibrary();
        $runner = $this->runner !== null
            ? $this->adaptProcessRunner(...)
            : null;

        \magelift_ensure_initial_database($magentoRoot, $this->tableCounter, $runner);
    }

    /**
     * @return array{host: string, dbname: string, username: string, password: string}
     */
    public function resolveDatabaseConnection(string $magentoRoot): array
    {
        self::loadRuntimeLibrary();

        return \magelift_initial_db_resolve_database_connection($magentoRoot);
    }

    public static function inlineRuntimeSource(): string
    {
        $source = file_get_contents(self::RUNTIME_SCRIPT);
        if ($source === false) {
            throw new RuntimeException('Initial database bootstrap script is missing.');
        }

        $source = preg_replace('/^#!.*\R/', '', $source);
        if (!is_string($source)) {
            throw new RuntimeException('Could not prepare the initial database bootstrap script.');
        }

        $source = preg_replace('/^<\?php\s*/', '', $source, 1);
        if (!is_string($source)) {
            throw new RuntimeException('Could not prepare the initial database bootstrap script.');
        }

        return trim($source)."\n\ntry { magelift_ensure_initial_database(getcwd() ?: '.'); }"
            ." catch (\\Throwable) { fwrite(STDERR, \"Initial database bootstrap failed. Check database configuration and connectivity.\\n\"); exit(1); }";
    }

    private static function loadRuntimeLibrary(): void
    {
        if (function_exists('magelift_ensure_initial_database')) {
            return;
        }

        if (!defined('MAGELIFT_INITIAL_DATABASE_BOOTSTRAP_LIBRARY')) {
            define('MAGELIFT_INITIAL_DATABASE_BOOTSTRAP_LIBRARY', true);
        }

        require_once self::RUNTIME_SCRIPT;
    }

    /** @param list<string> $argv */
    private function adaptProcessRunner(array $argv, string $cwd): int
    {
        if ($argv === []) {
            throw new RuntimeException('Magento subprocess argv is empty.');
        }

        $runner = $this->runner ?? new NativeProcessRunner();
        $result = $runner->run(new ProcessRequest(
            $argv,
            $cwd,
            timeoutSeconds: 1800.0,
        ));
        if (!$result->succeeded()) {
            $command = $argv[1] ?? 'subprocess';
            throw new RuntimeException(sprintf('%s failed during initial database bootstrap.', $command));
        }

        return 0;
    }
}
