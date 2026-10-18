<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Lifecycle;

use Closure;
use InvalidArgumentException;
use MageLift\Build\Lifecycle\HookRelationship;
use MageLift\Build\Lifecycle\LifecycleHook;
use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\Step;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class LifecycleHookTest extends TestCase
{
    #[DataProvider('invalidHooks')]
    public function testRejectsInvalidHooks(Closure $factory, string $message): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage($message);

        $factory();
    }

    public static function invalidHooks(): iterable
    {
        yield 'missing target' => [
            static fn () => new LifecycleHook(HookRelationship::Disable, ''),
            'Invalid lifecycle hook target step ID',
        ];
        yield 'command target' => [
            static fn () => new LifecycleHook(HookRelationship::Disable, 'bin/magento cache:flush'),
            'Invalid lifecycle hook target step ID',
        ];
        yield 'disable with step' => [
            static fn () => new LifecycleHook(
                HookRelationship::Disable,
                'build.compile',
                new Step('custom.compile', Phase::Build),
            ),
            'cannot define a step',
        ];
        yield 'relationship without step' => [
            static fn () => new LifecycleHook(HookRelationship::Before, 'build.compile'),
            'must define a step',
        ];
        yield 'self target' => [
            static fn () => new LifecycleHook(
                HookRelationship::Replace,
                'build.compile',
                new Step('build.compile', Phase::Build),
            ),
            'cannot target itself',
        ];
    }
}
