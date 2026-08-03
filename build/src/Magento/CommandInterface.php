<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

interface CommandInterface
{
    public function executable(): Executable;

    /** @return list<string> */
    public function arguments(): array;

    /** @return non-empty-list<string> */
    public function argv(): array;
}
