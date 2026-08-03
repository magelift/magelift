<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

use InvalidArgumentException;

final readonly class LifecycleHook
{
    private const ID_PATTERN = '/^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$/D';

    public function __construct(
        private HookRelationship $relationship,
        private string $targetStepId,
        private ?StepInterface $step = null,
    ) {
        if (preg_match(self::ID_PATTERN, $this->targetStepId) !== 1) {
            throw new InvalidArgumentException(sprintf('Invalid lifecycle hook target step ID "%s".', $this->targetStepId));
        }

        if ($this->relationship === HookRelationship::Disable && $this->step !== null) {
            throw new InvalidArgumentException('A disable hook cannot define a step.');
        }

        if ($this->relationship !== HookRelationship::Disable && $this->step === null) {
            throw new InvalidArgumentException(sprintf('A %s hook must define a step.', $this->relationship->value));
        }

        if ($this->step?->id() === $this->targetStepId) {
            throw new InvalidArgumentException('A lifecycle hook step cannot target itself.');
        }
    }

    public function relationship(): HookRelationship
    {
        return $this->relationship;
    }

    public function targetStepId(): string
    {
        return $this->targetStepId;
    }

    public function step(): ?StepInterface
    {
        return $this->step;
    }
}
