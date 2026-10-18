<?php

declare(strict_types=1);

namespace MageLift\Build\Process;

use RuntimeException;

final class ProcessFailed extends RuntimeException
{
    public function __construct(public readonly ProcessResult $result)
    {
        $reason = match (true) {
            $result->timedOut => 'timed out',
            $result->cancelled => 'was cancelled',
            default => sprintf('exited with code %d', $result->exitCode),
        };

        parent::__construct(sprintf('Process %s.', $reason));
    }
}
