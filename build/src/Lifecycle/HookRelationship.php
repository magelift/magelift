<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

enum HookRelationship: string
{
    case Before = 'before';
    case After = 'after';
    case Replace = 'replace';
    case Disable = 'disable';
}
