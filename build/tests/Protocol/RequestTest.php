<?php

declare(strict_types=1);

namespace MageLift\Build\Tests\Protocol;

use MageLift\Build\Protocol\FinalizeRequest;
use MageLift\Build\Protocol\InvalidProtocolRequest;
use MageLift\Build\Protocol\PrepareRequest;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class RequestTest extends TestCase
{
    public function testPrepareGoldenFixtureMatchesGoCodecByteForByte(): void
    {
        $json = self::prepareJson();
        $request = PrepareRequest::fromJson($json);

        self::assertSame('/workspace/shop', $request->repositoryRoot);
        self::assertSame('open-source', $request->application['edition']);
        self::assertSame('8.5.1', $request->phpVersion);
        self::assertSame(self::canonicalPrepareJson(), $request->toCanonicalJson());
    }

    public function testFinalizeGoldenFixtureMatchesGoCodecByteForByte(): void
    {
        $json = '{"protocolVersion":1,"stage":"finalize","finalize":{"preparedArtifact":"dist/rootfs.tar","sourceRevision":"'.str_repeat('a', 40).'","imageDigest":"sha256:'.str_repeat('b', 64).'"}}';
        $request = FinalizeRequest::fromJson($json);

        self::assertSame('dist/rootfs.tar', $request->preparedArtifact);
        self::assertSame($json, $request->toCanonicalJson());
    }

    public function testParsesAndCanonicalizesValidatedLifecycleHooks(): void
    {
        $json = str_replace(
            '"staticContent":[]',
            '"staticContent":[],"lifecycleHooks":[{"id":"build.prepare","phase":"build","relationship":"before","target":"build","command":{"executable":"composer","arguments":["run-script","prepare"]}}]',
            str_replace(
                '"staticContent":[{"locale":"fr_FR","theme":"Magento/luma"},{"locale":"en_US","theme":"Magento/luma"}]',
                '"staticContent":[]',
                self::prepareJson(),
            ),
        );
        $request = PrepareRequest::fromJson($json);

        self::assertSame('build.prepare', $request->lifecycleHooks[0]['id']);
        self::assertStringContainsString('"lifecycleHooks":[{', $request->toCanonicalJson());
    }

    #[DataProvider('invalidPrepareRequests')]
    public function testRejectsInvalidPrepareRequests(string $json, string $message): void
    {
        $this->expectException(InvalidProtocolRequest::class);
        $this->expectExceptionMessage($message);

        PrepareRequest::fromJson($json);
    }

    public static function invalidPrepareRequests(): iterable
    {
        yield 'unknown envelope key' => [self::replace('"stage"', '"unknown":true,"stage"'), 'unknown key'];
        yield 'both payloads' => [self::replace('"prepare":', '"finalize":{},"prepare":'), 'unknown key'];
        yield 'unknown application key' => [self::replace('"webRuntime":"nginx-fpm"', '"webRuntime":"nginx-fpm","secret":true'), 'unknown key'];
        yield 'wrong version' => [self::replace('"protocolVersion":1', '"protocolVersion":2'), 'protocolVersion'];
        yield 'wrong stage' => [self::replace('"stage":"prepare"', '"stage":"finalize"'), 'stage must be'];
        yield 'relative root' => [self::replace('/workspace/shop', 'workspace/shop'), 'absolute path'];
        yield 'unsafe input path' => [self::replace('app/etc/config.php', '../auth.json'), 'cannot traverse'];
        yield 'duplicate input path' => [self::replace('composer.lock', 'app/etc/config.php'), 'Duplicate build input'];
        yield 'secret value' => [self::replace('/workspace/shop', 'ssm://workspace'), 'forbidden secret'];
    }

    #[DataProvider('invalidFinalizeRequests')]
    public function testRejectsInvalidFinalizeRequests(string $json, string $message): void
    {
        $this->expectException(InvalidProtocolRequest::class);
        $this->expectExceptionMessage($message);

        FinalizeRequest::fromJson($json);
    }

    public static function invalidFinalizeRequests(): iterable
    {
        $valid = '{"protocolVersion":1,"stage":"finalize","finalize":{"preparedArtifact":"dist/rootfs.tar","sourceRevision":"'.str_repeat('a', 40).'","imageDigest":"sha256:'.str_repeat('b', 64).'"}}';
        yield 'tag' => [str_replace('sha256:'.str_repeat('b', 64), 'app:latest', $valid), 'OCI digest'];
        yield 'parent traversal' => [str_replace('dist/rootfs.tar', '../../rootfs.tar', $valid), 'cannot traverse'];
        yield 'unknown payload key' => [str_replace('"imageDigest":', '"manifestPath":"manifest.json","imageDigest":', $valid), 'unknown key'];
    }

    private static function prepareJson(): string
    {
        return '{"protocolVersion":1,"stage":"prepare","prepare":{"repositoryRoot":"/workspace/shop","sourceRevision":"'.str_repeat('a', 40).'","application":{"edition":"open-source","version":"2.4.9","mode":"integrated","webRuntime":"nginx-fpm"},"phpVersion":"8.5.1","compatibilityStatus":"supported","inputFiles":[{"path":"composer.lock","sha256":"'.str_repeat('b', 64).'"},{"path":"app/etc/config.php","sha256":"'.str_repeat('c', 64).'"}],"staticContent":[{"locale":"fr_FR","theme":"Magento/luma"},{"locale":"en_US","theme":"Magento/luma"}]}}';
    }

    private static function canonicalPrepareJson(): string
    {
        return '{"protocolVersion":1,"stage":"prepare","prepare":{"repositoryRoot":"/workspace/shop","sourceRevision":"'.str_repeat('a', 40).'","application":{"edition":"open-source","version":"2.4.9","mode":"integrated","webRuntime":"nginx-fpm"},"phpVersion":"8.5.1","compatibilityStatus":"supported","inputFiles":[{"path":"app/etc/config.php","sha256":"'.str_repeat('c', 64).'"},{"path":"composer.lock","sha256":"'.str_repeat('b', 64).'"}],"staticContent":[{"locale":"en_US","theme":"Magento/luma"},{"locale":"fr_FR","theme":"Magento/luma"}]}}';
    }

    private static function replace(string $search, string $replacement): string
    {
        return str_replace($search, $replacement, self::prepareJson());
    }
}
