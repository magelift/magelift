<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Magento\LifecyclePlan;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class LifecyclePlanTest extends TestCase
{
    #[DataProvider('phaseCommands')]
    public function testPlansExplicitArgumentVectors(Phase $phase, array $expected): void
    {
        $commands = (new LifecyclePlan())->commandsFor($phase);

        self::assertSame(
            $expected,
            array_map(static fn ($command): array => $command->argv(), $commands),
        );
    }

    public static function phaseCommands(): iterable
    {
        yield 'validate' => [Phase::Validate, [
            ['composer', 'validate', '--strict'],
            ['composer', 'check-platform-reqs', '--no-dev'],
        ]];
        yield 'build' => [Phase::Build, [
            ['composer', 'install', '--no-dev', '--prefer-dist', '--no-interaction', '--no-progress', '--optimize-autoloader'],
            ['bin/magento', 'setup:di:compile'],
            ['bin/magento', 'setup:static-content:deploy', '--no-interaction'],
        ]];
        yield 'package' => [Phase::Package, []];
        yield 'deploy' => [Phase::Deploy, [
            ['bin/magento', 'app:config:import', '--no-interaction'],
            ['bin/magento', 'setup:upgrade', '--keep-generated', '--no-interaction'],
            ['bin/magento', 'cache:clean'],
        ]];
        yield 'post-deploy' => [Phase::PostDeploy, [
            ['bin/magento', 'cache:flush'],
        ]];
    }

    public function testPlanContainsNoEnvironmentSpecificValues(): void
    {
        $plan = new LifecyclePlan();
        $argv = [];
        foreach (Phase::cases() as $phase) {
            foreach ($plan->commandsFor($phase) as $command) {
                array_push($argv, ...$command->argv());
            }
        }

        self::assertNotContains('production', $argv);
        self::assertNotContains('staging', $argv);
        self::assertNotContains('AWS_REGION', $argv);
    }

    public function testPlansConfiguredStaticContentMatrix(): void
    {
        $plan = new LifecyclePlan([], [
            ['locale' => 'en_US', 'theme' => 'Magento/blank'],
            ['locale' => 'fr_FR', 'theme' => 'Vendor/theme'],
        ]);

        self::assertSame([
            ['composer', 'install', '--no-dev', '--prefer-dist', '--no-interaction', '--no-progress', '--optimize-autoloader'],
            ['bin/magento', 'setup:di:compile'],
            ['bin/magento', 'setup:static-content:deploy', '--language', 'en_US', '--theme', 'Magento/blank', '--no-interaction'],
            ['bin/magento', 'setup:static-content:deploy', '--language', 'fr_FR', '--theme', 'Vendor/theme', '--no-interaction'],
        ], array_map(
            static fn ($command): array => $command->argv(),
            $plan->commandsFor(Phase::Build),
        ));
    }

    public function testPreparationStepsHaveStableDependencyOrder(): void
    {
        $steps = (new LifecyclePlan())->steps();

        self::assertSame(['validate', 'build', 'package'], array_map(
            static fn ($step): string => $step->id(),
            $steps,
        ));
        self::assertSame([], $steps[0]->dependencies());
        self::assertSame(['validate'], $steps[1]->dependencies());
        self::assertSame(['build'], $steps[2]->dependencies());
    }

    public function testDeploymentStepsHaveStableDependencyOrder(): void
    {
        $steps = (new LifecyclePlan())->deploymentSteps();

        self::assertSame(['deploy', 'post-deploy'], array_map(
            static fn ($step): string => $step->id(),
            $steps,
        ));
        self::assertSame(['package'], $steps[0]->dependencies());
        self::assertSame(['deploy'], $steps[1]->dependencies());
        self::assertSame(Phase::Deploy, $steps[0]->phase());
        self::assertSame(Phase::PostDeploy, $steps[1]->phase());
    }
}
