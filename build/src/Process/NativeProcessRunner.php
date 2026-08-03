<?php

declare(strict_types=1);

namespace MageLift\Build\Process;

use RuntimeException;

final class NativeProcessRunner implements ProcessRunner
{
    public function run(ProcessRequest $request): ProcessResult
    {
        $descriptors = [
            0 => ['pipe', 'r'],
            1 => ['pipe', 'w'],
            2 => ['pipe', 'w'],
        ];
        $pipes = [];
        $process = proc_open(
            $request->argv,
            $descriptors,
            $pipes,
            $request->workingDirectory,
            $request->environment,
            ['bypass_shell' => true],
        );
        if (!is_resource($process)) {
            throw new RuntimeException('Unable to start process.');
        }

        fclose($pipes[0]);
        stream_set_blocking($pipes[1], false);
        stream_set_blocking($pipes[2], false);
        $stdout = '';
        $stderr = '';
        $startedAt = hrtime(true);
        $timedOut = false;
        $cancelled = false;
		$observedExitCode = -1;

        while (true) {
            $stdout .= stream_get_contents($pipes[1]);
            $stderr .= stream_get_contents($pipes[2]);
            $status = proc_get_status($process);
            if (!$status['running']) {
                $observedExitCode = $status['exitcode'];
                break;
            }

            $cancelled = $request->isCancelled !== null && ($request->isCancelled)();
            $timedOut = !$cancelled
                && (hrtime(true) - $startedAt) / 1_000_000_000 >= $request->timeoutSeconds;
            if ($cancelled || $timedOut) {
                proc_terminate($process);
                usleep(10_000);
                if (proc_get_status($process)['running']) {
                    proc_terminate($process, 9);
                }
            }

            usleep(10_000);
        }

        $stdout .= stream_get_contents($pipes[1]);
        $stderr .= stream_get_contents($pipes[2]);
        fclose($pipes[1]);
        fclose($pipes[2]);
        $closedExitCode = proc_close($process);
		$exitCode = $observedExitCode >= 0 ? $observedExitCode : $closedExitCode;
        if ($exitCode < 0 || (($timedOut || $cancelled) && $exitCode === 0)) {
            $exitCode = 1;
        }

        return new ProcessResult($exitCode, $stdout, $stderr, $timedOut, $cancelled);
    }
}
