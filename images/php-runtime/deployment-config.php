<?php

declare(strict_types=1);

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

$encoded = json_encode($configuration, JSON_UNESCAPED_SLASHES);
if ($encoded !== false) {
    putenv('MAGENTO_DC__OVERRIDE=' . $encoded);
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
