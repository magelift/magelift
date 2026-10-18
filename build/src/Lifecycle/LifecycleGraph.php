<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

final class LifecycleGraph
{
    /** @var array<string, StepInterface> */
    private array $steps = [];

    /**
     * @param iterable<StepInterface> $steps
     * @param iterable<LifecycleHook> $hooks
     */
    public function __construct(iterable $steps, iterable $hooks = [])
    {
        foreach ($steps as $step) {
            if (isset($this->steps[$step->id()])) {
                throw new InvalidLifecycleGraph(sprintf('Duplicate step ID "%s".', $step->id()));
            }

            $this->steps[$step->id()] = $step;
        }

        foreach ($hooks as $hook) {
            $this->applyHook($hook);
        }

        $this->validateReferences();
        $this->orderedSteps();
    }

    private function applyHook(LifecycleHook $hook): void
    {
        $targetId = $hook->targetStepId();
        $target = $this->steps[$targetId] ?? null;
        if ($target === null) {
            throw new InvalidLifecycleGraph(sprintf(
                'Lifecycle hook targets unknown step "%s".',
                $targetId,
            ));
        }

        if ($hook->relationship() === HookRelationship::Disable) {
            $this->rewireDependencies($targetId, $target->dependencies());
            unset($this->steps[$targetId]);

            return;
        }

        $step = $hook->step();
        if ($step === null) {
            throw new InvalidLifecycleGraph('Lifecycle hook step is missing.');
        }
        if (isset($this->steps[$step->id()])) {
            throw new InvalidLifecycleGraph(sprintf('Duplicate step ID "%s".', $step->id()));
        }

        switch ($hook->relationship()) {
            case HookRelationship::Before:
                $this->steps[$step->id()] = $this->withDependencies(
                    $step,
                    [...$target->dependencies(), ...$step->dependencies()],
                );
                $this->steps[$targetId] = $this->withDependencies($target, [$step->id()]);
                break;

            case HookRelationship::After:
                $this->rewireDependencies($targetId, [$step->id()]);
                $this->steps[$step->id()] = $this->withDependencies(
                    $step,
                    [$targetId, ...$step->dependencies()],
                );
                break;

            case HookRelationship::Replace:
                $this->rewireDependencies($targetId, [$step->id()]);
                unset($this->steps[$targetId]);
                $this->steps[$step->id()] = $this->withDependencies(
                    $step,
                    [...$target->dependencies(), ...$step->dependencies()],
                );
                break;

            case HookRelationship::Disable:
                // Handled before a step is required.
                break;
        }
    }

    /** @param list<string> $replacementIds */
    private function rewireDependencies(string $targetId, array $replacementIds): void
    {
        foreach ($this->steps as $id => $step) {
            if ($id === $targetId || !in_array($targetId, $step->dependencies(), true)) {
                continue;
            }

            $dependencies = [];
            foreach ($step->dependencies() as $dependency) {
                array_push($dependencies, ...($dependency === $targetId ? $replacementIds : [$dependency]));
            }
            $this->steps[$id] = $this->withDependencies($step, $dependencies);
        }
    }

    /** @param list<string> $dependencies */
    private function withDependencies(StepInterface $step, array $dependencies): StepInterface
    {
        return new ConfiguredStep($step, array_values(array_unique($dependencies)));
    }

    /** @return list<StepInterface> */
    public function orderedSteps(): array
    {
        $states = [];
        $ordered = [];

        foreach (array_keys($this->steps) as $id) {
            $this->visit($id, $states, $ordered);
        }

        return $ordered;
    }

    private function validateReferences(): void
    {
        foreach ($this->steps as $step) {
            foreach ($step->dependencies() as $dependency) {
                if ($dependency === $step->id()) {
                    throw new InvalidLifecycleGraph(sprintf('Step "%s" cannot depend on itself.', $step->id()));
                }

                if (!isset($this->steps[$dependency])) {
                    throw new InvalidLifecycleGraph(sprintf(
                        'Step "%s" depends on unknown step "%s".',
                        $step->id(),
                        $dependency,
                    ));
                }
            }
        }
    }

    /**
     * @param array<string, 1|2> $states
     * @param list<StepInterface> $ordered
     */
    private function visit(string $id, array &$states, array &$ordered): void
    {
        if (($states[$id] ?? null) === 2) {
            return;
        }

        if (($states[$id] ?? null) === 1) {
            throw new InvalidLifecycleGraph(sprintf('Lifecycle dependency cycle includes step "%s".', $id));
        }

        $states[$id] = 1;
        foreach ($this->steps[$id]->dependencies() as $dependency) {
            $this->visit($dependency, $states, $ordered);
        }
        $states[$id] = 2;
        $ordered[] = $this->steps[$id];
    }
}
