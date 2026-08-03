<?php

declare(strict_types=1);

namespace MageLift\Build\Process;

interface ProcessRunner
{
    public function run(ProcessRequest $request): ProcessResult;
}
