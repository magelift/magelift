<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use MageLift\Build\Artifact\ArtifactManifest;
use MageLift\Build\Artifact\CompatibilityStatus;
use MageLift\Build\Artifact\MagentoEdition;
use MageLift\Build\Protocol\FinalizeRequest;
use MageLift\Build\Protocol\FinalizeResponse;
use MageLift\Build\Protocol\InvalidProtocolRequest;
use MageLift\Build\Protocol\PrepareRequest;
use MageLift\Build\Protocol\PrepareResponse;
use MageLift\Build\Protocol\ProtocolJson;
use RuntimeException;

final readonly class RunnerService
{
    public function __construct(
        private RunnerFilesystem $filesystem,
        private RevisionVerifier $revisionVerifier,
        private Preparation $preparation,
        private string $workspaceRoot,
        private string $mageLiftVersion,
        private string $buildPackageVersion,
    ) {
        if (!str_starts_with($this->workspaceRoot, '/')) {
            throw new RuntimeException('Runner workspace must be an absolute path.');
        }
    }

    public function handle(string $json): string
    {
        $envelope = ProtocolJson::decodeObject($json);
        return match ($envelope['stage'] ?? null) {
            'prepare' => $this->prepare(PrepareRequest::fromJson($json))->toCanonicalJson(),
            'finalize' => $this->finalize(FinalizeRequest::fromJson($json))->toCanonicalJson(),
            default => throw new InvalidProtocolRequest('Protocol stage must be "prepare" or "finalize".'),
        };
    }

    private function prepare(PrepareRequest $request): PrepareResponse
    {
        if ($this->isWithin($this->workspaceRoot, $request->repositoryRoot)) {
            throw new RuntimeException('Runner workspace must be outside the source repository.');
        }
        if (!$this->revisionVerifier->matches($request->repositoryRoot, $request->sourceRevision)) {
            throw new RuntimeException('Source revision does not match the immutable build request.');
        }
        foreach ($request->inputFiles as $input) {
            $actual = $this->filesystem->checksumWithin($request->repositoryRoot, $input['path']);
            if (!hash_equals($input['sha256'], $actual)) {
                throw new RuntimeException('Immutable build input checksum mismatch.');
            }
        }

        $output = $this->preparation->prepare($request);
        $preparedArtifact = 'prepared/'.$request->sourceRevision.'.json';
        $response = new PrepareResponse(
            $preparedArtifact,
            $output->phpVersion,
            $output->phpExtensions,
            $output->enabledModules,
            $output->checksums,
            $output->requiredRuntimeCapabilities,
        );
        $metadata = ProtocolJson::encode([
            'sourceRevision' => $request->sourceRevision,
            'application' => $request->application,
            'phpVersion' => $output->phpVersion,
            'compatibilityStatus' => $request->compatibilityStatus,
            'phpExtensions' => $output->phpExtensions,
            'enabledModules' => $output->enabledModules,
            'inputFiles' => $request->inputFiles,
            'staticContent' => $request->staticContent,
            'checksums' => $output->checksums,
            'requiredRuntimeCapabilities' => $output->requiredRuntimeCapabilities,
            'mageLiftVersion' => $this->mageLiftVersion,
            'buildPackageVersion' => $this->buildPackageVersion,
        ]);
        $this->filesystem->atomicWrite($this->workspacePath($preparedArtifact), $metadata, 0o600);

        return $response;
    }

    private function finalize(FinalizeRequest $request): FinalizeResponse
    {
        $metadata = $this->metadata($this->filesystem->read($this->workspacePath($request->preparedArtifact)));
        if (!hash_equals($metadata['sourceRevision'], $request->sourceRevision)) {
            throw new RuntimeException('Finalize source revision does not match prepared metadata.');
        }

        $buildInputs = [];
        foreach ($metadata['inputFiles'] as $input) {
            $buildInputs[$input['path']] = $input['sha256'];
        }
        $checksums = [];
        foreach ($metadata['checksums'] as $checksum) {
            $checksums[$checksum['path']] = $checksum['sha256'];
        }
        $staticContent = [];
        foreach ($metadata['staticContent'] as $content) {
            $staticContent[$content['locale']][] = $content['theme'];
        }
        $manifest = new ArtifactManifest(
            $request->sourceRevision,
            $request->imageDigest,
            $metadata['mageLiftVersion'],
            $metadata['buildPackageVersion'],
            MagentoEdition::from($metadata['application']['edition']),
            $metadata['application']['version'],
            $metadata['phpVersion'],
            $metadata['phpExtensions'],
            $metadata['enabledModules'],
            $buildInputs,
            $checksums,
            $staticContent,
            $metadata['requiredRuntimeCapabilities'],
            CompatibilityStatus::from($metadata['compatibilityStatus']),
        );
        $manifestPath = 'manifests/'.$request->sourceRevision.'.json';
        $contents = $manifest->toCanonicalJson();
        $absolutePath = $this->workspacePath($manifestPath);
        $this->filesystem->atomicWrite($absolutePath, $contents, 0o644);

        return new FinalizeResponse($request->imageDigest, $manifestPath, hash('sha256', $contents));
    }

    /** @return array<string, mixed> */
    private function metadata(string $json): array
    {
        $value = ProtocolJson::decodeObject($json);
        ProtocolJson::assertKeys($value, [
            'sourceRevision', 'application', 'phpVersion', 'compatibilityStatus', 'phpExtensions', 'enabledModules', 'inputFiles',
            'staticContent', 'checksums', 'requiredRuntimeCapabilities', 'mageLiftVersion', 'buildPackageVersion',
        ], [
            'sourceRevision', 'application', 'phpVersion', 'compatibilityStatus', 'phpExtensions', 'enabledModules', 'inputFiles',
            'staticContent', 'checksums', 'requiredRuntimeCapabilities', 'mageLiftVersion', 'buildPackageVersion',
        ], 'prepared metadata');
        ProtocolJson::assertNoSecrets($value);

        return $value;
    }

    private function workspacePath(string $relativePath): string
    {
        return rtrim($this->workspaceRoot, '/').'/'.$relativePath;
    }

    private function isWithin(string $path, string $root): bool
    {
        $root = rtrim($root, '/');
        return $path === $root || str_starts_with($path, $root.'/');
    }
}
