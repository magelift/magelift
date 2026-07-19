<?php

declare(strict_types=1);

namespace MageLift\Build\Artifact;

enum MagentoEdition: string
{
    case OpenSource = 'open-source';
    case Commerce = 'commerce';
}
