<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

enum ProtocolOperation: string
{
    case Prepare = 'prepare';
    case Finalize = 'finalize';
}
