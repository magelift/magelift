<?php

declare(strict_types=1);

/**
 * @param null|callable(array{host: string, dbname: string, username: string, password: string}): int $tableCounter
 * @param null|callable(list<string>, string): int $processRunner
 */
function magelift_ensure_initial_database(
    string $magentoRoot,
    ?callable $tableCounter = null,
    ?callable $processRunner = null,
): void {
}

/**
 * @return array{host: string, dbname: string, username: string, password: string}
 */
function magelift_initial_db_resolve_database_connection(string $magentoRoot): array
{
    return ['host' => '', 'dbname' => '', 'username' => '', 'password' => ''];
}
