<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

enum Phase: string
{
    case Validate = 'validate';
    case Build = 'build';
    case Package = 'package';
    case Deploy = 'deploy';
    case PostDeploy = 'post-deploy';
}
