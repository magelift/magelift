<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use MageLift\Build\Protocol\PrepareRequest;

final class MagentoCapabilityProvider implements RequiredCapabilityProvider
{
    public function capabilitiesFor(PrepareRequest $request): array
    {
        $capabilities = ['cache.valkey', 'database.mysql', 'media.object-storage', 'search.opensearch'];
        sort($capabilities, SORT_STRING);

        return $capabilities;
    }
}
