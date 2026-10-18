<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use RuntimeException;

/**
 * Static content deploy resolves the default website from config.php.
 * A composer skeleton has modules only. A shop that already dumped its
 * stores is left untouched. The scaffold is the single-store shape
 * Magento writes on a fresh install, so compile does not need a database.
 */
final class DefaultStoreScaffold
{
    public const COMMAND = '/opt/magelift/build/bin/ensure-default-store.php';

    public function ensure(string $configPath): void
    {
        if (!is_file($configPath)) {
            throw new RuntimeException('app/etc/config.php is missing.');
        }
        $config = require $configPath;
        if (!is_array($config)) {
            throw new RuntimeException('app/etc/config.php must return an array.');
        }

        $scopes = is_array($config['scopes'] ?? null) ? $config['scopes'] : [];
        $websites = is_array($scopes['websites'] ?? null) ? $scopes['websites'] : [];
        if ($websites !== []) {
            if (!$this->hasDefaultWebsite($websites)) {
                throw new RuntimeException('config.php defines websites but none is the default. Static content deploy needs one default website.');
            }

            return;
        }

        $config['scopes'] = array_replace($scopes, $this->defaultScopes());
        $exported = "<?php\nreturn ".$this->export($config).";\n";
        $temporary = $configPath.'.magelift-tmp';
        if (file_put_contents($temporary, $exported) === false) {
            throw new RuntimeException('Could not write the default store scaffold.');
        }
        if (!rename($temporary, $configPath)) {
            @unlink($temporary);
            throw new RuntimeException('Could not replace app/etc/config.php with the default store scaffold.');
        }
    }

    /** @param array<mixed, mixed> $websites */
    private function hasDefaultWebsite(array $websites): bool
    {
        foreach ($websites as $website) {
            if (!is_array($website)) {
                continue;
            }
            if ((string) ($website['is_default'] ?? '0') === '1') {
                return true;
            }
        }

        return false;
    }

    /** @return array<string, array<mixed, array<string, string>>> */
    private function defaultScopes(): array
    {
        return [
            'websites' => [
                'admin' => [
                    'website_id' => '0',
                    'code' => 'admin',
                    'name' => 'Admin',
                    'sort_order' => '0',
                    'default_group_id' => '0',
                    'is_default' => '0',
                ],
                'base' => [
                    'website_id' => '1',
                    'code' => 'base',
                    'name' => 'Main Website',
                    'sort_order' => '0',
                    'default_group_id' => '1',
                    'is_default' => '1',
                ],
            ],
            'groups' => [
                0 => [
                    'group_id' => '0',
                    'website_id' => '0',
                    'code' => 'default',
                    'name' => 'Default',
                    'root_category_id' => '0',
                    'default_store_id' => '0',
                ],
                1 => [
                    'group_id' => '1',
                    'website_id' => '1',
                    'code' => 'main_website_store',
                    'name' => 'Main Website Store',
                    'root_category_id' => '2',
                    'default_store_id' => '1',
                ],
            ],
            'stores' => [
                'admin' => [
                    'store_id' => '0',
                    'code' => 'admin',
                    'website_id' => '0',
                    'group_id' => '0',
                    'name' => 'Admin',
                    'sort_order' => '0',
                    'is_active' => '1',
                ],
                'default' => [
                    'store_id' => '1',
                    'code' => 'default',
                    'website_id' => '1',
                    'group_id' => '1',
                    'name' => 'Default Store View',
                    'sort_order' => '0',
                    'is_active' => '1',
                ],
            ],
        ];
    }

    /** @param array<mixed, mixed> $value */
    private function export(array $value): string
    {
        $exported = var_export($value, true);
        if (!is_string($exported)) {
            throw new RuntimeException('Could not export app/etc/config.php.');
        }

        return $exported;
    }
}
