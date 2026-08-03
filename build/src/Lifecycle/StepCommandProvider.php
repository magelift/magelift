<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

use MageLift\Build\Magento\CommandInterface;

interface StepCommandProvider
{
    /** @return list<CommandInterface> */
    public function commandsForStep(StepInterface $step): array;
}
