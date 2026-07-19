<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use RuntimeException;

final class ConfigModuleReader
{
    /** @return list<string> */
    public function enabledModules(string $configPath): array
    {
        $source = @file_get_contents($configPath);
        if ($source === false) {
            throw new RuntimeException('Prepared Magento config.php is missing.');
        }
        $allowedTokens = [T_OPEN_TAG, T_RETURN, T_ARRAY, T_WHITESPACE, T_CONSTANT_ENCAPSED_STRING, T_DOUBLE_ARROW, T_LNUMBER, T_COMMENT, T_DOC_COMMENT];
        foreach (token_get_all($source) as $token) {
            if (is_array($token)) {
                if (!in_array($token[0], $allowedTokens, true)) {
                    throw new RuntimeException('Magento config.php contains non-literal PHP.');
                }
            } elseif (!str_contains('[](),;', $token)) {
                throw new RuntimeException('Magento config.php contains an invalid literal token.');
            }
        }
        $config = (static fn (string $path): mixed => include $path)($configPath);
        if (!is_array($config) || !isset($config['modules']) || !is_array($config['modules'])) {
            throw new RuntimeException('Magento config.php does not contain a modules map.');
        }
        $modules = [];
        foreach ($config['modules'] as $module => $enabled) {
            if (!is_string($module) || !is_int($enabled) || !in_array($enabled, [0, 1], true)) {
                throw new RuntimeException('Magento config.php contains an invalid module entry.');
            }
            if ($enabled === 1) {
                $modules[] = $module;
            }
        }
        if ($modules === []) {
            throw new RuntimeException('Magento config.php contains no enabled modules.');
        }
        sort($modules, SORT_STRING);

        return $modules;
    }
}
