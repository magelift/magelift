<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

final class GitRevisionVerifier implements RevisionVerifier
{
    public function matches(string $repositoryRoot, string $sourceRevision): bool
    {
        $pipes = [];
        $process = @proc_open(
            ['git', '-c', 'safe.directory='.$repositoryRoot, '-C', $repositoryRoot, 'rev-parse', 'HEAD'],
            [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']],
            $pipes,
            options: ['bypass_shell' => true],
        );
        if (!is_resource($process)) {
            return false;
        }
        fclose($pipes[0]);
        $revision = trim((string) stream_get_contents($pipes[1]));
        stream_get_contents($pipes[2]);
        fclose($pipes[1]);
        fclose($pipes[2]);

        return proc_close($process) === 0 && hash_equals($sourceRevision, $revision);
    }
}
