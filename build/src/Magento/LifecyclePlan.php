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
     * @param null|list<CommandInterface> $patchCommands Resolved patch lifecycle commands
     */
    public function __construct(
        private array $hookCommands = [],
        private array $staticContent = [],
        private array $hotfixPatches = [],
        private bool $refreshModules = false,
        private ?array $patchCommands = null,
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
                // Adobe's supported source tags contain Composer warnings that
                // do not make the package invalid. Keep schema errors fatal.
                // Adobe release locks can carry a Composer-version-specific
                // content hash. The lock remains an immutable build input and
                // `composer install` below still resolves from that exact lock.
                new Command(Executable::Composer, ['validate', '--no-check-lock']),
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
                ...$this->moduleRefreshCommands(),
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
        if ($this->patchCommands !== null) {
            return $this->patchCommands;
        }
        if ($this->hotfixPatches === []) {
            return [];
        }

        return PatchApplier::commands($this->hotfixPatches);
    }

    /** @return list<CommandInterface> */
    private function moduleRefreshCommands(): array
    {
        if (!$this->refreshModules) {
            return [];
        }

        // A source release without app/etc/config.php has no module state to preserve.
        // Generate the standard Adobe module map before compilation instead of
        // changing the explicit enablement of an existing project.
        return [new Command(Executable::Magento, ['module:enable', '--all'])];
    }

    /** @return list<CommandInterface> */
    private function staticContentCommands(): array
    {
        if ($this->staticContent === []) {
            // The build runner has no Magento database. SCD is opt-in because
            // locales and websites must be dumped into config.php first.
            return [];
        }

        $commands = [];
        foreach ($this->staticContent as $content) {
            if ($content['locale'] === '' || $content['theme'] === '') {
                throw new InvalidArgumentException('Static content locale and theme must be non-empty.');
            }
            $args = [
                'setup:static-content:deploy',
                '--force',
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
