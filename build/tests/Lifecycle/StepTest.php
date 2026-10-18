<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Lifecycle;

use InvalidArgumentException;
use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\RetryPolicy;
use MageLift\Build\Lifecycle\Step;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class StepTest extends TestCase
{
    #[DataProvider('invalidIds')]
    public function testRejectsUnstableIds(string $id): void
    {
        $this->expectException(InvalidArgumentException::class);

        new Step($id, Phase::Build);
    }

    public static function invalidIds(): iterable
    {
        yield 'empty' => [''];
        yield 'uppercase' => ['Build.compile'];
        yield 'spaces' => ['build compile'];
        yield 'shell command' => ['bin/magento setup:di:compile'];
        yield 'trailing separator' => ['build.compile-'];
    }

    public function testRejectsDuplicateDependencies(): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage('duplicate dependencies');

        new Step('build.compile', Phase::Build, ['validate.config', 'validate.config']);
    }

    public function testRetriesRequireIdempotency(): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage('idempotent');

        new RetryPolicy(2);
    }

    public function testAcceptsRetryForIdempotentStep(): void
    {
        $policy = new RetryPolicy(3, 5, true);
        $step = new Step('build.assets', Phase::Build, retries: $policy);

        self::assertSame($policy, $step->retryPolicy());
    }
}
