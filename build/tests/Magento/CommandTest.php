<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use InvalidArgumentException;
use MageLift\Build\Magento\Command;
use MageLift\Build\Magento\Executable;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class CommandTest extends TestCase
{
    public function testPreservesArgumentBoundaries(): void
    {
        $command = new Command(Executable::Magento, ['config:set', 'design/theme/theme_id', 'Vendor theme']);

        self::assertSame(
            ['bin/magento', 'config:set', 'design/theme/theme_id', 'Vendor theme'],
            $command->argv(),
        );
    }

    #[DataProvider('invalidArguments')]
    public function testRejectsInvalidArguments(mixed $argument): void
    {
        $this->expectException(InvalidArgumentException::class);

        new Command(Executable::Composer, [$argument]);
    }

    public static function invalidArguments(): iterable
    {
        yield 'empty' => [''];
        yield 'null byte' => ["install\0--no-dev"];
        yield 'non-string' => [42];
    }
}
