<?php

declare(strict_types=1);

const MAGELIFT_INITIAL_DB_SKIP_PREFIX = 'SKIP_SCHEMA_DATA_BOOTSTRAP';
const MAGELIFT_INITIAL_DB_AUTO_PREPEND = '/usr/local/lib/magelift/deployment-config.php';

/** @var list<string> */
const MAGELIFT_INITIAL_DB_SCHEMA_DATA_COMMANDS = [
    'setup:db-schema:upgrade',
    'setup:db-data:upgrade',
];

/**
 * @param null|callable(array{host: string, dbname: string, username: string, password: string}): int $tableCounter
 * @param null|callable(list<string>, string): int $processRunner
 */
function magelift_ensure_initial_database(
    string $magentoRoot,
    ?callable $tableCounter = null,
    ?callable $processRunner = null,
): void {
    $root = magelift_initial_db_resolve_magento_root($magentoRoot);
    magelift_initial_db_load_runtime_deployment_override($root);

    $connection = magelift_initial_db_resolve_database_connection($root);
    $tableCount = magelift_initial_db_count_tables($connection, $tableCounter);
    if ($tableCount > 0) {
        echo MAGELIFT_INITIAL_DB_SKIP_PREFIX.' table_count='.$tableCount."\n";

        return;
    }

    $magento = $root.'/bin/magento';
    if (!is_file($magento)) {
        magelift_initial_db_fail('bin/magento is missing.');
    }

    $runner = $processRunner ?? 'magelift_initial_db_run_process';
    foreach (MAGELIFT_INITIAL_DB_SCHEMA_DATA_COMMANDS as $command) {
        $exitCode = $runner([$magento, $command, '--no-interaction'], $root);
        if ($exitCode !== 0) {
            fwrite(STDERR, $command.' failed during initial database bootstrap (exit '.$exitCode.").\n");
            exit($exitCode);
        }
    }
}

/**
 * @return array{host: string, dbname: string, username: string, password: string}
 */
function magelift_initial_db_resolve_database_connection(string $magentoRoot): array
{
    $root = magelift_initial_db_resolve_magento_root($magentoRoot);
    $envFile = $root.'/app/etc/env.php';
    if (!is_file($envFile)) {
        magelift_initial_db_fail('app/etc/env.php is missing.');
    }

    $configuration = require $envFile;
    if (!is_array($configuration)) {
        magelift_initial_db_fail('app/etc/env.php must return an array.');
    }

    $override = getenv('MAGENTO_DC__OVERRIDE');
    if ($override !== false && trim($override) !== '') {
        $decoded = json_decode($override, true);
        if (!is_array($decoded)) {
            magelift_initial_db_fail('MAGENTO_DC__OVERRIDE must decode to a JSON object.');
        }
        $configuration = magelift_initial_db_merge_deployment_config(
            $configuration,
            magelift_initial_db_resolve_deployment_config($decoded),
        );
    }

    $configuration = magelift_initial_db_resolve_deployment_config($configuration);
    $default = $configuration['db']['connection']['default'] ?? null;
    if (!is_array($default)) {
        magelift_initial_db_fail('Database connection default is not configured.');
    }

    return [
        'host' => magelift_initial_db_non_empty_string($default['host'] ?? null, 'host'),
        'dbname' => magelift_initial_db_non_empty_string($default['dbname'] ?? null, 'dbname'),
        'username' => magelift_initial_db_non_empty_string($default['username'] ?? null, 'username'),
        'password' => magelift_initial_db_non_empty_string($default['password'] ?? null, 'password'),
    ];
}

function magelift_initial_db_load_runtime_deployment_override(string $magentoRoot): void
{
    if (!is_file(MAGELIFT_INITIAL_DB_AUTO_PREPEND)) {
        return;
    }

    $previousDirectory = getcwd();
    if ($previousDirectory === false || !chdir($magentoRoot)) {
        magelift_initial_db_fail('Could not enter the Magento root.');
    }

    try {
        require_once MAGELIFT_INITIAL_DB_AUTO_PREPEND;
    } finally {
        if ($previousDirectory !== false) {
            chdir($previousDirectory);
        }
    }
}

function magelift_initial_db_resolve_magento_root(string $magentoRoot): string
{
    $resolved = realpath($magentoRoot);
    if ($resolved === false || !is_dir($resolved)) {
        magelift_initial_db_fail('Magento root is not a directory.');
    }

    return $resolved;
}

/**
 * @param array{host: string, dbname: string, username: string, password: string} $connection
 * @param null|callable(array{host: string, dbname: string, username: string, password: string}): int $tableCounter
 */
function magelift_initial_db_count_tables(array $connection, ?callable $tableCounter = null): int
{
    if ($tableCounter !== null) {
        return $tableCounter($connection);
    }

    $dsn = sprintf(
        'mysql:host=%s;dbname=%s;charset=utf8mb4',
        $connection['host'],
        $connection['dbname'],
    );

    try {
        $pdo = new PDO($dsn, $connection['username'], $connection['password'], [
            PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
        ]);
    } catch (PDOException) {
        magelift_initial_db_fail('Could not connect to the Magento database.');
    }

    $statement = $pdo->query('SHOW TABLES');
    if ($statement === false) {
        magelift_initial_db_fail('Could not list database tables.');
    }

    $tables = $statement->fetchAll(PDO::FETCH_COLUMN);
    if (!is_array($tables)) {
        magelift_initial_db_fail('Could not read database tables.');
    }

    return count($tables);
}

/** @param array<mixed, mixed> $base @param array<mixed, mixed> $overlay */
function magelift_initial_db_merge_deployment_config(array $base, array $overlay): array
{
    foreach ($overlay as $key => $value) {
        if (is_array($value) && isset($base[$key]) && is_array($base[$key])) {
            $base[$key] = magelift_initial_db_merge_deployment_config($base[$key], $value);
            continue;
        }
        $base[$key] = $value;
    }

    return $base;
}

/** @param array<mixed, mixed> $value */
function magelift_initial_db_resolve_deployment_config(array $value): array
{
    foreach ($value as $key => $child) {
        if (is_array($child)) {
            $value[$key] = magelift_initial_db_resolve_deployment_config($child);
            continue;
        }
        if (is_string($child)) {
            $value[$key] = magelift_initial_db_resolve_env_placeholder($child);
        }
    }

    return $value;
}

function magelift_initial_db_resolve_env_placeholder(string $value): string
{
    $pattern = '~^#env\(\s*(?<name>\w+)\s*(,\s*"(?<default>[^"]*)")?\)$~';
    if (preg_match($pattern, $value, $matches) !== 1) {
        return $value;
    }

    $environmentValue = getenv($matches['name']);
    if ($environmentValue !== false) {
        return $environmentValue;
    }

    return array_key_exists('default', $matches) ? $matches['default'] : '';
}

function magelift_initial_db_non_empty_string(mixed $value, string $field): string
{
    if (!is_string($value) || $value === '') {
        magelift_initial_db_fail(sprintf('Database connection %s is not configured.', $field));
    }

    return $value;
}

/** @param list<string> $argv */
function magelift_initial_db_run_process(array $argv, string $cwd): int
{
    // Inherit the CLI streams: draining two blocking pipes sequentially can
    // deadlock when either Magento stream fills its pipe buffer.
    $process = proc_open($argv, [0 => STDIN, 1 => STDOUT, 2 => STDERR], $pipes, $cwd);
    if (!is_resource($process)) {
        magelift_initial_db_fail('Could not start a Magento subprocess.');
    }

    return proc_close($process);
}

function magelift_initial_db_fail(string $message): never
{
    if (defined('MAGELIFT_INITIAL_DATABASE_BOOTSTRAP_LIBRARY')) {
        throw new RuntimeException($message);
    }

    fwrite(STDERR, $message."\n");
    exit(1);
}
