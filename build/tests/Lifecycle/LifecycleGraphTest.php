<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Lifecycle;

use MageLift\Build\Lifecycle\InvalidLifecycleGraph;
use MageLift\Build\Lifecycle\HookRelationship;
use MageLift\Build\Lifecycle\LifecycleGraph;
use MageLift\Build\Lifecycle\LifecycleHook;
use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\Step;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class LifecycleGraphTest extends TestCase
{
    public function testOrdersDependenciesBeforeConsumersDeterministically(): void
    {
        $graph = new LifecycleGraph([
            new Step('package.image', Phase::Package, ['build.compile', 'build.assets']),
            new Step('build.assets', Phase::Build, ['validate.config']),
            new Step('validate.config', Phase::Validate),
            new Step('build.compile', Phase::Build, ['validate.config']),
        ]);

        self::assertSame(
            ['validate.config', 'build.compile', 'build.assets', 'package.image'],
            array_map(static fn ($step): string => $step->id(), $graph->orderedSteps()),
        );
    }

    #[DataProvider('invalidGraphs')]
    public function testRejectsInvalidGraphs(array $steps, string $message): void
    {
        $this->expectException(InvalidLifecycleGraph::class);
        $this->expectExceptionMessage($message);

        new LifecycleGraph($steps);
    }

    public static function invalidGraphs(): iterable
    {
        yield 'duplicate ID' => [
            [new Step('build.compile', Phase::Build), new Step('build.compile', Phase::Build)],
            'Duplicate step ID',
        ];
        yield 'unknown dependency' => [
            [new Step('build.compile', Phase::Build, ['validate.config'])],
            'depends on unknown step',
        ];
        yield 'self dependency' => [
            [new Step('build.compile', Phase::Build, ['build.compile'])],
            'cannot depend on itself',
        ];
        yield 'cycle' => [
            [
                new Step('build.first', Phase::Build, ['build.second']),
                new Step('build.second', Phase::Build, ['build.first']),
            ],
            'dependency cycle',
        ];
    }

    #[DataProvider('hookTransformations')]
    public function testAppliesHookTransformations(
        LifecycleHook $hook,
        array $expectedOrder,
        array $expectedPackageDependencies,
    ): void {
        $graph = new LifecycleGraph([
            new Step('validate.config', Phase::Validate),
            new Step('build.compile', Phase::Build, ['validate.config']),
            new Step('package.image', Phase::Package, ['build.compile']),
        ], [$hook]);

        $steps = $graph->orderedSteps();
        self::assertSame($expectedOrder, array_map(static fn ($step): string => $step->id(), $steps));

        $package = array_values(array_filter(
            $steps,
            static fn ($step): bool => $step->id() === 'package.image',
        ))[0];
        self::assertSame($expectedPackageDependencies, $package->dependencies());
    }

    public static function hookTransformations(): iterable
    {
        yield 'before' => [
            new LifecycleHook(
                HookRelationship::Before,
                'build.compile',
                new Step('custom.prepare', Phase::Build),
            ),
            ['validate.config', 'custom.prepare', 'build.compile', 'package.image'],
            ['build.compile'],
        ];
        yield 'after' => [
            new LifecycleHook(
                HookRelationship::After,
                'build.compile',
                new Step('custom.verify', Phase::Build),
            ),
            ['validate.config', 'build.compile', 'custom.verify', 'package.image'],
            ['custom.verify'],
        ];
        yield 'replace' => [
            new LifecycleHook(
                HookRelationship::Replace,
                'build.compile',
                new Step('custom.compile', Phase::Build),
            ),
            ['validate.config', 'custom.compile', 'package.image'],
            ['custom.compile'],
        ];
        yield 'disable' => [
            new LifecycleHook(HookRelationship::Disable, 'build.compile'),
            ['validate.config', 'package.image'],
            ['validate.config'],
        ];
    }

    public function testRejectsHookForUnknownTarget(): void
    {
        $this->expectException(InvalidLifecycleGraph::class);
        $this->expectExceptionMessage('targets unknown step');

        new LifecycleGraph(
            [new Step('build.compile', Phase::Build)],
            [new LifecycleHook(
                HookRelationship::Before,
                'build.missing',
                new Step('custom.prepare', Phase::Build),
            )],
        );
    }

    public function testRejectsHookStepIdCollision(): void
    {
        $this->expectException(InvalidLifecycleGraph::class);
        $this->expectExceptionMessage('Duplicate step ID');

        new LifecycleGraph(
            [
                new Step('build.compile', Phase::Build),
                new Step('custom.prepare', Phase::Build),
            ],
            [new LifecycleHook(
                HookRelationship::Before,
                'build.compile',
                new Step('custom.prepare', Phase::Build),
            )],
        );
    }

    public function testValidatesCyclesIntroducedByHooks(): void
    {
        $this->expectException(InvalidLifecycleGraph::class);
        $this->expectExceptionMessage('dependency cycle');

        new LifecycleGraph(
            [
                new Step('build.compile', Phase::Build),
                new Step('package.image', Phase::Package, ['build.compile']),
            ],
            [new LifecycleHook(
                HookRelationship::Before,
                'build.compile',
                new Step('custom.prepare', Phase::Build, ['package.image']),
            )],
        );
    }
}
