<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

final readonly class FinalizeResponse
{
    public function __construct(
        public string $imageDigest,
        public string $manifestPath,
        public string $manifestSha256,
    ) {
        ProtocolJson::digest($this->imageDigest);
        ProtocolJson::relativePath($this->manifestPath, 'manifest');
        ProtocolJson::checksum($this->manifestSha256, 'manifest');
        ProtocolJson::assertNoSecrets([
            'imageDigest' => $this->imageDigest,
            'manifestPath' => $this->manifestPath,
            'manifestSha256' => $this->manifestSha256,
        ]);
    }

    public function toCanonicalJson(): string
    {
        return ProtocolJson::encode([
            'protocolVersion' => ProtocolJson::VERSION,
            'stage' => ProtocolOperation::Finalize->value,
            'finalize' => [
                'imageDigest' => $this->imageDigest,
                'manifestPath' => $this->manifestPath,
                'manifestSha256' => $this->manifestSha256,
            ],
        ]);
    }
}
