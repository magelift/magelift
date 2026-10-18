<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

use MageLift\Build\Artifact\BuildRecipe;

final readonly class FinalizeRequest
{
    private function __construct(
        public string $preparedArtifact,
        public string $sourceRevision,
        public string $imageDigest,
        public ?BuildRecipe $buildRecipe = null,
    ) {
    }

    public static function fromJson(string $json): self
    {
        $envelope = ProtocolJson::decodeObject($json);
        ProtocolJson::assertKeys($envelope, ['protocolVersion', 'stage', 'finalize'], ['protocolVersion', 'stage', 'finalize']);
        ProtocolJson::assertEnvelope($envelope, ProtocolOperation::Finalize);
        $payload = ProtocolJson::object($envelope['finalize'], 'finalize');
        ProtocolJson::assertKeys(
            $payload,
            ['preparedArtifact', 'sourceRevision', 'imageDigest', 'buildRecipe'],
            ['preparedArtifact', 'sourceRevision', 'imageDigest'],
            'finalize',
        );
        try {
            ProtocolJson::assertNoSecrets($payload);
        } catch (ProtocolException $error) {
            throw new InvalidProtocolRequest('Finalize payload contains a forbidden secret.', previous: $error);
        }

        $buildRecipe = null;
        if (array_key_exists('buildRecipe', $payload)) {
            try {
                $buildRecipe = BuildRecipe::fromArray(ProtocolJson::object($payload['buildRecipe'], 'finalize.buildRecipe'), 'finalize.buildRecipe');
            } catch (\InvalidArgumentException $error) {
                throw new InvalidProtocolRequest($error->getMessage(), previous: $error);
            }
        }

        return new self(
            ProtocolJson::relativePath($payload['preparedArtifact'], 'prepared artifact'),
            ProtocolJson::revision($payload['sourceRevision']),
            ProtocolJson::digest($payload['imageDigest']),
            $buildRecipe,
        );
    }

    public function toCanonicalJson(): string
    {
        $finalize = [
            'preparedArtifact' => $this->preparedArtifact,
            'sourceRevision' => $this->sourceRevision,
            'imageDigest' => $this->imageDigest,
        ];
        if ($this->buildRecipe !== null) {
            $finalize['buildRecipe'] = $this->buildRecipe->toArray();
        }

        return ProtocolJson::encode([
            'protocolVersion' => ProtocolJson::VERSION,
            'stage' => ProtocolOperation::Finalize->value,
            'finalize' => $finalize,
        ]);
    }
}
