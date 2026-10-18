<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use MageLift\Build\Lifecycle\Phase;

interface PlanInterface
{
    /** @return list<CommandInterface> */
    public function commandsFor(Phase $phase): array;
}
