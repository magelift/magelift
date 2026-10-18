<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Magento;

use MageLift\Build\Magento\PatchApplier;
use MageLift\Build\Magento\PatchCommand;
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

    public function testReapplyingSyntheticPatchIsIdempotent(): void
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

        $applier = new PatchApplier(new NativeProcessRunner());
        $applier->apply($root);
        $applier->apply($root);

        self::assertSame('hello patched', rtrim((string) file_get_contents($root.'/app/sample.txt')));
        self::assertFileDoesNotExist($root.'/app/sample.txt.rej');
    }

    public function testCorruptPatchFailsLoud(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/app', 0o700);
        file_put_contents($root.'/app/sample.txt', "hello world\n");
        mkdir($root.'/m2-hotfixes', 0o700);
        file_put_contents($root.'/m2-hotfixes/bad.patch', "this is not a unified diff\n");

        try {
            (new PatchApplier(new NativeProcessRunner()))->apply($root);
            self::fail('Expected hotfix apply failure.');
        } catch (RuntimeException $error) {
            self::assertStringContainsString('m2-hotfixes hotfix "m2-hotfixes/bad.patch"', $error->getMessage());
            self::assertStringContainsString('locked Magento version', $error->getMessage());
            self::assertStringNotContainsString('this is not a unified diff', $error->getMessage());
        }
    }

    public function testConflictedHunkFailsWithoutMutatingTargetOrCreatingRejectArtifacts(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/app', 0o700);
        $target = $root.'/app/two.txt';
        $original = "alpha\nbeta\ngamma\n";
        file_put_contents($target, $original);
        mkdir($root.'/m2-hotfixes', 0o700);
        file_put_contents($root.'/m2-hotfixes/conflict.patch', <<<'PATCH'
--- a/app/two.txt
+++ b/app/two.txt
@@ -1,3 +1,3 @@
-NOTFOUND
+ALPHA
 beta
 gamma

PATCH);

        try {
            (new PatchApplier(new NativeProcessRunner()))->apply($root);
            self::fail('Expected hotfix apply failure.');
        } catch (RuntimeException $error) {
            self::assertStringContainsString('m2-hotfixes/conflict.patch', $error->getMessage());
            self::assertStringNotContainsString('NOTFOUND', $error->getMessage());
        }

        // The lifecycle runs PatchCommand directly rather than PatchApplier::apply.
        $command = new PatchCommand('m2-hotfixes/conflict.patch');
        $result = $command->run(new NativeProcessRunner(), new ProcessRequest($command->argv(), $root, [], 300));
        self::assertFalse($result->succeeded());
        self::assertStringContainsString('m2-hotfixes/conflict.patch', $result->stderr);
        self::assertStringContainsString('locked Magento version before retrying', $result->stderr);
        self::assertStringNotContainsString('NOTFOUND', $result->stderr);
        self::assertSame($original, (string) file_get_contents($target));
        self::assertNoPatchRejectArtifacts($root);
    }

    public function testPartiallyAppliedTwoHunkPatchFailsWithoutRejectArtifacts(): void
    {
        $root = $this->tempProject();
        mkdir($root.'/app', 0o700);
        $target = $root.'/app/two.txt';
        $pristine = "alpha\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nomega\nline10\n";
        $partial = str_replace("alpha\n", "ALPHA\n", $pristine);
        mkdir($root.'/m2-hotfixes', 0o700);
        file_put_contents($root.'/m2-hotfixes/two-hunk.patch', <<<'PATCH'
--- a/app/two.txt
+++ b/app/two.txt
@@ -1,2 +1,2 @@
-alpha
+ALPHA
 line2
@@ -9,2 +9,2 @@
-omega
+OMEGA
 line10

PATCH);

        // Prove this is a valid two-hunk patch, not a malformed-diff failure.
        file_put_contents($target, $pristine);
        $applier = new PatchApplier(new NativeProcessRunner());
        $applier->apply($root);
        self::assertSame(str_replace("omega\n", "OMEGA\n", $partial), (string) file_get_contents($target));
        file_put_contents($target, $partial);

        try {
            (new PatchApplier(new NativeProcessRunner()))->apply($root);
            self::fail('Expected hotfix apply failure.');
        } catch (RuntimeException $error) {
            self::assertStringContainsString('m2-hotfixes/two-hunk.patch', $error->getMessage());
            self::assertStringContainsString('locked Magento version', $error->getMessage());
            self::assertStringNotContainsString('ALPHA', $error->getMessage());
        }

        self::assertSame($partial, (string) file_get_contents($target));
        self::assertNoPatchRejectArtifacts($root);
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

    private static function assertNoPatchRejectArtifacts(string $root): void
    {
        $iterator = new \RecursiveIteratorIterator(
            new \RecursiveDirectoryIterator($root, \FilesystemIterator::SKIP_DOTS),
        );
        foreach ($iterator as $file) {
            if (!$file->isFile()) {
                continue;
            }
            $basename = $file->getBasename();
            self::assertFalse(
                str_ends_with($basename, '.rej') || str_ends_with($basename, '.orig'),
                sprintf('Unexpected patch artifact "%s".', $file->getPathname()),
            );
        }
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
