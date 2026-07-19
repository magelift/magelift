<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

final readonly class FinalizeRequest
{
    private function __construct(
        public string $preparedArtifact,
        public string $sourceRevision,
        public string $imageDigest,
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
            ['preparedArtifact', 'sourceRevision', 'imageDigest'],
            ['preparedArtifact', 'sourceRevision', 'imageDigest'],
            'finalize',
        );
        try {
            ProtocolJson::assertNoSecrets($payload);
        } catch (ProtocolException $error) {
            throw new InvalidProtocolRequest('Finalize payload contains a forbidden secret.', previous: $error);
        }

        return new self(
            ProtocolJson::relativePath($payload['preparedArtifact'], 'prepared artifact'),
            ProtocolJson::revision($payload['sourceRevision']),
            ProtocolJson::digest($payload['imageDigest']),
        );
    }

    public function toCanonicalJson(): string
    {
        return ProtocolJson::encode([
            'protocolVersion' => ProtocolJson::VERSION,
            'stage' => ProtocolOperation::Finalize->value,
            'finalize' => [
                'preparedArtifact' => $this->preparedArtifact,
                'sourceRevision' => $this->sourceRevision,
                'imageDigest' => $this->imageDigest,
            ],
        ]);
    }
}
