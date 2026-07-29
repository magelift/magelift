<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\Step;
use MageLift\Build\Lifecycle\StepCommandProvider;
use MageLift\Build\Lifecycle\StepInterface;
use InvalidArgumentException;

final class LifecyclePlan implements PlanInterface, StepCommandProvider
{
    /**
     * @param array<string, list<CommandInterface>> $hookCommands
     * @param list<array{locale: string, theme: string, strategy?: string, threads?: int}> $staticContent
     * @param list<string> $hotfixPatches Relative m2-hotfixes/*.patch paths (alpha-sorted by caller)
     */
    public function __construct(
        private array $hookCommands = [],
        private array $staticContent = [],
        private array $hotfixPatches = [],
    )
    {
    }

    /**
     * Return the nodes executed while preparing an immutable application image.
     * Deployment nodes are exposed separately because preparation runs without
     * runtime credentials or a Magento database connection.
     *
     * @return list<StepInterface>
     */
    public function steps(): array
    {
        return [
            new Step('validate', Phase::Validate, timeout: 300),
            new Step('build', Phase::Build, ['validate'], timeout: 1800),
            // OCI packaging is owned by the Go pipeline. Keeping this node in the
            // graph gives the full lifecycle a stable hand-off point.
            new Step('package', Phase::Package, ['build'], timeout: 300),
        ];
    }

    /** @return list<StepInterface> */
    public function deploymentSteps(): array
    {
        return [
            new Step('deploy', Phase::Deploy, ['package'], timeout: 1800),
            new Step('post-deploy', Phase::PostDeploy, ['deploy'], timeout: 600),
        ];
    }

    /** @return list<CommandInterface> */
    public function commandsForStep(StepInterface $step): array
    {
        if (isset($this->hookCommands[$step->id()])) {
            return $this->hookCommands[$step->id()];
        }

        return $this->commandsFor($step->phase());
    }

    /** @return list<CommandInterface> */
    public function commandsFor(Phase $phase): array
    {
        return match ($phase) {
            Phase::Validate => [
                new Command(Executable::Composer, ['validate', '--strict']),
                new Command(Executable::Composer, ['check-platform-reqs', '--no-dev']),
            ],
            Phase::Build => [
                new Command(Executable::Composer, [
                    'install',
                    '--no-dev',
                    '--prefer-dist',
                    '--no-interaction',
                    '--no-progress',
                    '--optimize-autoloader',
                ]),
                ...$this->hotfixPatchCommands(),
                new Command(Executable::Magento, ['setup:di:compile']),
                ...$this->staticContentCommands(),
            ],
            Phase::Package => [],
            Phase::Deploy => [
                new Command(Executable::Magento, ['app:config:import', '--no-interaction']),
                new Command(Executable::Magento, ['setup:upgrade', '--keep-generated', '--no-interaction']),
                new Command(Executable::Magento, ['cache:clean']),
            ],
            Phase::PostDeploy => [
                new Command(Executable::Magento, ['cache:flush']),
            ],
        };
    }

    /** @return list<CommandInterface> */
    private function hotfixPatchCommands(): array
    {
        if ($this->hotfixPatches === []) {
            return [];
        }

        return PatchApplier::commands($this->hotfixPatches);
    }

    /** @return list<CommandInterface> */
    private function staticContentCommands(): array
    {
        if ($this->staticContent === []) {
            return [new Command(Executable::Magento, ['setup:static-content:deploy', '--no-interaction'])];
        }

        $commands = [];
        foreach ($this->staticContent as $content) {
            if ($content['locale'] === '' || $content['theme'] === '') {
                throw new InvalidArgumentException('Static content locale and theme must be non-empty.');
            }
            $args = [
                'setup:static-content:deploy',
                '--language',
                $content['locale'],
                '--theme',
                $content['theme'],
            ];
            $strategy = $content['strategy'] ?? '';
            if (is_string($strategy) && $strategy !== '') {
                $args[] = '-s';
                $args[] = $strategy;
            }
            $threads = $content['threads'] ?? 0;
            if (is_int($threads) && $threads > 0) {
                $args[] = '-j';
                $args[] = (string) $threads;
            }
            $args[] = '--no-interaction';
            $commands[] = new Command(Executable::Magento, $args);
        }

        return $commands;
    }
}
