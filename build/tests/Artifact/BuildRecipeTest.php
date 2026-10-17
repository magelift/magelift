<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Artifact;

use InvalidArgumentException;
use MageLift\Build\Artifact\BuildRecipe;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class BuildRecipeTest extends TestCase
{
    public function testToArraySortsBaseImageRoles(): void
    {
        $recipe = self::recipe(
            baseImages: [
                'PHP_BASE' => 'ghcr.io/magelift/php@sha256:'.str_repeat('1', 64),
                'NGINX_BASE' => 'ghcr.io/magelift/nginx@sha256:'.str_repeat('3', 64),
                'COMPOSER_BASE' => 'ghcr.io/magelift/composer@sha256:'.str_repeat('2', 64),
            ],
        );

        self::assertSame(
            [
                'sha256' => str_repeat('a', 64),
                'platform' => 'linux/arm64',
                'baseImages' => [
                    'COMPOSER_BASE' => 'ghcr.io/magelift/composer@sha256:'.str_repeat('2', 64),
                    'NGINX_BASE' => 'ghcr.io/magelift/nginx@sha256:'.str_repeat('3', 64),
                    'PHP_BASE' => 'ghcr.io/magelift/php@sha256:'.str_repeat('1', 64),
                ],
            ],
            $recipe->toArray(),
        );
    }

    #[DataProvider('invalidRecipes')]
    public function testRejectsInvalidRecipes(array $overrides, string $message): void
    {
        $this->expectException(InvalidArgumentException::class);
        $this->expectExceptionMessage($message);

        $data = self::recipeData();
        foreach ($overrides as $key => $value) {
            if ($key === 'baseImagesReplace') {
                $data['baseImages'] = $value;
                continue;
            }
            if ($key === 'baseImages' && is_array($value)) {
                $data['baseImages'] = array_merge($data['baseImages'], $value);
                continue;
            }
            $data[$key] = $value;
        }
        BuildRecipe::fromArray($data);
    }

    public static function invalidRecipes(): iterable
    {
        yield 'malformed digest prefix' => [['sha256' => 'sha256:'.str_repeat('a', 64)], 'lowercase SHA-256 checksum'];
        yield 'uppercase digest' => [['sha256' => strtoupper(str_repeat('a', 64))], 'lowercase SHA-256 checksum'];
        yield 'invalid platform' => [['platform' => 'linux/ppc64le'], 'linux/amd64 or linux/arm64'];
        yield 'missing base role' => [[
            'baseImagesReplace' => [
                'PHP_BASE' => 'ghcr.io/magelift/php@sha256:'.str_repeat('1', 64),
                'COMPOSER_BASE' => 'ghcr.io/magelift/composer@sha256:'.str_repeat('2', 64),
            ],
        ], 'missing required role'];
        yield 'unknown base role' => [['baseImages' => ['RUNTIME_BASE' => 'ghcr.io/magelift/runtime@sha256:'.str_repeat('4', 64)]], 'unknown role'];
        yield 'unpinned base ref' => [['baseImages' => ['PHP_BASE' => 'ghcr.io/magelift/php:latest']], 'registry-qualified'];
        yield 'url base ref' => [['baseImages' => ['PHP_BASE' => 'https://ghcr.io/magelift/php@sha256:'.str_repeat('1', 64)]], 'must not be a URL'];
        yield 'unknown field' => [['extra' => true], 'unknown key'];
    }

    /** @param array<string, string> $baseImages */
    private static function recipe(
        string $sha256 = '',
        string $platform = 'linux/arm64',
        array $baseImages = [],
    ): BuildRecipe {
        $data = self::recipeData();
        if ($sha256 !== '') {
            $data['sha256'] = $sha256;
        }
        if ($platform !== '') {
            $data['platform'] = $platform;
        }
        if ($baseImages !== []) {
            $data['baseImages'] = array_merge($data['baseImages'], $baseImages);
        }

        return BuildRecipe::fromArray($data);
    }

    /** @return array<string, mixed> */
    private static function recipeData(): array
    {
        return [
            'sha256' => str_repeat('a', 64),
            'platform' => 'linux/amd64',
            'baseImages' => [
                'PHP_BASE' => 'ghcr.io/magelift/php@sha256:'.str_repeat('1', 64),
                'COMPOSER_BASE' => 'ghcr.io/magelift/composer@sha256:'.str_repeat('2', 64),
                'NGINX_BASE' => 'ghcr.io/magelift/nginx@sha256:'.str_repeat('3', 64),
            ],
        ];
    }
}
