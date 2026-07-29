<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

enum Executable: string
{
    case Composer = 'composer';
    case Magento = 'bin/magento';
    case Patch = 'patch';
}
