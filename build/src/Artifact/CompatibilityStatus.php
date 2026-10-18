<?php

declare(strict_types=1);

namespace MageLift\Build\Artifact;

enum CompatibilityStatus: string
{
    case Supported = 'supported';
    case UnsupportedAllowed = 'unsupported-allowed';
}
