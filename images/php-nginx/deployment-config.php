<?php

declare(strict_types=1);

// nginx and PHP-FPM use this file to read Magento runtime settings.
// Handle CORS before Magento bootstrap, including on preflight requests.
mageliftHandleCors();

// Magento resolves #env(...) placeholders only when a scalar path is read.
// Build a complete runtime override before Magento bootstraps so consumers of
// a parent section, such as db/connection/default, receive resolved values too.
$envFile = '/app/app/etc/env.php';
if (!is_file($envFile)) {
    return;
}

$configuration = require $envFile;
if (!is_array($configuration)) {
    return;
}

$credentialEnv = getenv('MAGELIFT_LOCAL_EMAIL_CREDENTIAL_ENV');
if ($credentialEnv !== false && preg_match('/\A[A-Z][A-Z0-9_]*\z/D', $credentialEnv) === 1) {
    $credential = getenv($credentialEnv);
    if ($credential !== false) {
        putenv('MAGELIFT_LOCAL_EMAIL_PASSWORD=' . $credential);
    }
}

$configuration = mageliftResolveDeploymentConfig($configuration);
$override = getenv('MAGENTO_DC__OVERRIDE');
if ($override !== false && trim($override) !== '') {
    $decoded = json_decode($override, true);
    if (is_array($decoded)) {
        $decoded = mageliftResolveDeploymentConfig($decoded);
        $configuration = mageliftMergeDeploymentConfig($configuration, $decoded);
    }
}

$configuration = mageliftNormalizeAmqpSSL($configuration);

// Azure MySQL requires TLS. Passing a CA also enables mysqlnd's certificate
// and hostname verification; never permit an override to disable it.
$databaseTLSCA = getenv('MAGELIFT_DATABASE_TLS_CA');
if ($databaseTLSCA !== false && $databaseTLSCA !== '') {
    if (!is_file($databaseTLSCA) || !is_readable($databaseTLSCA)
        || !defined('PDO::MYSQL_ATTR_SSL_CA') || !defined('PDO::MYSQL_ATTR_SSL_VERIFY_SERVER_CERT')) {
        throw new RuntimeException('Database TLS CA is unavailable.');
    }
    $caOption = defined('Pdo\\Mysql::ATTR_SSL_CA') ? constant('Pdo\\Mysql::ATTR_SSL_CA') : constant('PDO::MYSQL_ATTR_SSL_CA');
    $verifyOption = defined('Pdo\\Mysql::ATTR_SSL_VERIFY_SERVER_CERT') ? constant('Pdo\\Mysql::ATTR_SSL_VERIFY_SERVER_CERT') : constant('PDO::MYSQL_ATTR_SSL_VERIFY_SERVER_CERT');
    $configuration['db']['connection']['default']['driver_options'][$caOption] = $databaseTLSCA;
    $configuration['db']['connection']['default']['driver_options'][$verifyOption] = true;
}

$encoded = json_encode($configuration, JSON_UNESCAPED_SLASHES);
if ($encoded !== false) {
    putenv('MAGENTO_DC__OVERRIDE=' . $encoded);
}

// Magento's AMQP factory uses strict boolean client setters.
function mageliftNormalizeAmqpSSL(array $configuration): array
{
    if (isset($configuration['queue']['amqp']['ssl_options']) && is_array($configuration['queue']['amqp']['ssl_options'])) {
        foreach (['verify_peer', 'verify_peer_name'] as $option) {
            if (isset($configuration['queue']['amqp']['ssl_options'][$option])) {
                $verified = filter_var(
                    $configuration['queue']['amqp']['ssl_options'][$option],
                    FILTER_VALIDATE_BOOLEAN,
                    FILTER_NULL_ON_FAILURE,
                );
                if ($verified === null) {
                    throw new RuntimeException('Invalid AMQP TLS verification setting.');
                }
                $configuration['queue']['amqp']['ssl_options'][$option] = $verified;
            }
        }
    }
    return $configuration;
}

function mageliftResolveDeploymentConfig(mixed $value): mixed
{
    if (is_array($value)) {
        foreach ($value as $key => $child) {
            $value[$key] = mageliftResolveDeploymentConfig($child);
        }
        return $value;
    }

    if (!is_string($value)) {
        return $value;
    }

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

function mageliftMergeDeploymentConfig(array $base, array $overlay): array
{
    foreach ($overlay as $key => $value) {
        if (is_array($value) && isset($base[$key]) && is_array($base[$key])) {
            $base[$key] = mageliftMergeDeploymentConfig($base[$key], $value);
            continue;
        }
        $base[$key] = $value;
    }

    return $base;
}

function mageliftHandleCors(): void
{
    if (PHP_SAPI === 'cli') {
        return;
    }
    $configured = getenv('CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ALLOWED_ORIGINS');
    if ($configured === false || trim($configured) === '') {
        return;
    }
    header('Vary: Origin', false);
    $origin = $_SERVER['HTTP_ORIGIN'] ?? '';
    $allowed = false;
    foreach (explode(',', $configured) as $candidate) {
        $candidate = rtrim(trim($candidate), '/');
        // Treat malformed configuration, wildcard entries and "null" as denied.
        $parts = parse_url($candidate);
        if (!is_array($parts) || !in_array($parts['scheme'] ?? '', ['http', 'https'], true)
            || empty($parts['host']) || str_contains($candidate, '*')
            || isset($parts['user']) || isset($parts['pass'])
            || isset($parts['query']) || isset($parts['fragment']) || isset($parts['path'])
            || preg_match('/[\x00-\x20\x7f]/', $candidate) === 1) {
            continue;
        }
        if ($origin === $candidate) {
            $allowed = true;
            break;
        }
    }
    $preflight = ($_SERVER['REQUEST_METHOD'] ?? '') === 'OPTIONS'
        && isset($_SERVER['HTTP_ACCESS_CONTROL_REQUEST_METHOD']);
    if (!$allowed) {
        if ($preflight) {
            http_response_code(403);
            exit;
        }
        return;
    }
    header('Access-Control-Allow-Origin: ' . $origin);
    header('Access-Control-Allow-Credentials: true');
    if (!$preflight) {
        return;
    }
    header('Vary: Access-Control-Request-Method', false);
    header('Vary: Access-Control-Request-Headers', false);
    $methods = ['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE'];
    $method = $_SERVER['HTTP_ACCESS_CONTROL_REQUEST_METHOD'];
    if (!in_array($method, $methods, true)) {
        http_response_code(403);
        exit;
    }
    $requestedHeaders = $_SERVER['HTTP_ACCESS_CONTROL_REQUEST_HEADERS'] ?? '';
    if (strlen($requestedHeaders) > 8192) {
        http_response_code(403);
        exit;
    }
    if ($requestedHeaders !== '') {
        foreach (explode(',', $requestedHeaders) as $requestedHeader) {
            if (preg_match('/\A[!#$%&\x27*+.^_`|~0-9A-Za-z-]+\z/D', trim($requestedHeader)) !== 1) {
                http_response_code(403);
                exit;
            }
        }
        header('Access-Control-Allow-Headers: ' . $requestedHeaders);
    }
    header('Access-Control-Allow-Methods: ' . implode(', ', $methods));
    http_response_code(204);
    exit;
}
