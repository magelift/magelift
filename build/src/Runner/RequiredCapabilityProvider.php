<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use MageLift\Build\Protocol\PrepareRequest;

interface RequiredCapabilityProvider
{
    /** @return list<string> */
    public function capabilitiesFor(PrepareRequest $request): array;
}
