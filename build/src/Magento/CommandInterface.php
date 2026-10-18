<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;

interface CommandInterface
{
    public function executable(): Executable;

    /** @return list<string> */
    public function arguments(): array;

    /** @return non-empty-list<string> */
    public function argv(): array;

    public function run(ProcessRunner $runner, ProcessRequest $request): ProcessResult;
}
