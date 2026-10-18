<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use MageLift\Build\Protocol\PrepareRequest;

interface Preparation
{
    public function prepare(PrepareRequest $request): PreparationOutput;
}
