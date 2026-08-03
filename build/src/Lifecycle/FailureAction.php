<?php

declare(strict_types=1);

namespace MageLift\Build\Lifecycle;

enum FailureAction: string
{
    case Abort = 'abort';
    case Continue = 'continue';
}
