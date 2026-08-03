<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Magento\PatchApplier;
use MageLift\Build\Process\NativeProcessRunner;
use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;
use PHPUnit\Framework\TestCase;
use RuntimeException;

final class PatchApplierTest extends TestCase
{
    public function testEmptyHotfixDirectoryIsNoOpSuccess(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/m2-hotfixes', 0o700);
        $runner = new RecordingRunner();

        (new PatchApplier($runner))->apply($root);

        self::assertSame([], $runner->requests);
        self::assertSame([], PatchApplier::discover($root));
    }

    public function testMissingHotfixDirectoryIsNoOpSuccess(): void
    {
        $root = $this->tempProject();
        $runner = new RecordingRunner();

        (new PatchApplier($runner))->apply($root);

        self::assertSame([], $runner->requests);
    }

    public function testDiscoversPatchesInAlphabeticalOrder(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/m2-hotfixes', 0o700);
        file_put_contents($root.'/m2-hotfixes/b-second.patch', "diff\n");
        file_put_contents($root.'/m2-hotfixes/a-first.patch', "diff\n");
        file_put_contents($root.'/m2-hotfixes/readme.txt', 'ignored');

        self::assertSame([
            'm2-hotfixes/a-first.patch',
            'm2-hotfixes/b-second.patch',
        ], PatchApplier::discover($root));
    }

    public function testAppliesSyntheticPatchAndUpdatesTargetFile(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/app', 0o700);
        file_put_contents($root.'/app/sample.txt', "hello world\n");
        mkdir($root.'/m2-hotfixes', 0o700);
        file_put_contents($root.'/m2-hotfixes/01-sample.patch', <<<'PATCH'
--- a/app/sample.txt
+++ b/app/sample.txt
@@ -1 +1 @@
-hello world
+hello patched

PATCH);

        (new PatchApplier(new NativeProcessRunner()))->apply($root);

        self::assertSame('hello patched', rtrim((string) file_get_contents($root.'/app/sample.txt')));
    }

    public function testCorruptPatchFailsLoud(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/app', 0o700);
        file_put_contents($root.'/app/sample.txt', "hello world\n");
        mkdir($root.'/m2-hotfixes', 0o700);
        file_put_contents($root.'/m2-hotfixes/bad.patch', "this is not a unified diff\n");

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('Failed to apply hotfix patch "m2-hotfixes/bad.patch"');
        (new PatchApplier(new NativeProcessRunner()))->apply($root);
    }

    public function testRejectsPathTraversalInCommandList(): void
    {
        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('must stay under m2-hotfixes');
        PatchApplier::commands(['m2-hotfixes/../evil.patch']);
    }

    public function testRejectsSymlinkedHotfixPatches(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/m2-hotfixes', 0o700);
        $outside = dirname($root).'/outside.patch';
        file_put_contents($outside, "diff\n");
        symlink($outside, $root.'/m2-hotfixes/linked.patch');

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('symbolic link');
        PatchApplier::discover($root);
    }

    public function testCommandsEmitPatchArgv(): void
    {
        $commands = PatchApplier::commands(['m2-hotfixes/01-fix.patch']);

        self::assertSame([
            ['patch', '-p1', '--forward', '--batch', '-i', 'm2-hotfixes/01-fix.patch'],
        ], array_map(static fn ($command): array => $command->argv(), $commands));
    }

    private function tempProject(): string
    {
        $root = sys_get_temp_dir().'/magelift-patch-'.bin2hex(random_bytes(6));
        mkdir($root, 0o700, true);

        return $root;
    }
}

final class RecordingRunner implements ProcessRunner
{
    /** @var list<ProcessRequest> */
    public array $requests = [];

    public function run(ProcessRequest $request): ProcessResult
    {
        $this->requests[] = $request;

        return new ProcessResult(0, '', '');
    }
}
