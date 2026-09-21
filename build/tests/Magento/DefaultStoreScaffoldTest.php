<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Magento\DefaultStoreScaffold;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class DefaultStoreScaffoldTest extends TestCase
{
    public function testWritesTheSingleStoreScaffoldWhenNoWebsiteExists(): void
    {
        $path = $this->config("<?php\nreturn ['modules' => ['Magento_Store' => 1]];\n");

        (new DefaultStoreScaffold())->ensure($path);

        $config = require $path;
        self::assertSame(1, $config['modules']['Magento_Store']);
        self::assertSame('1', $config['scopes']['websites']['base']['is_default']);
        self::assertSame('1', $config['scopes']['stores']['default']['store_id']);
        self::assertSame('1', $config['scopes']['groups'][1]['default_store_id']);
    }

    public function testLeavesADumpedDefaultWebsiteUntouched(): void
    {
        $original = "<?php\nreturn ['scopes' => ['websites' => ['base' => ['code' => 'base', 'is_default' => '1']]]];\n";
        $path = $this->config($original);

        (new DefaultStoreScaffold())->ensure($path);

        self::assertSame($original, (string) file_get_contents($path));
    }

    public function testRejectsWebsitesWithNoDefault(): void
    {
        $path = $this->config("<?php\nreturn ['scopes' => ['websites' => ['extra' => ['code' => 'extra', 'is_default' => '0']]]];\n");

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('none is the default');

        (new DefaultStoreScaffold())->ensure($path);
    }

    private function config(string $contents): string
    {
        $directory = sys_get_temp_dir().'/magelift-scaffold-'.bin2hex(random_bytes(4));
        mkdir($directory.'/app/etc', 0o755, true);
        $path = $directory.'/app/etc/config.php';
        file_put_contents($path, $contents);

        return $path;
    }
}
