<?php

declare(strict_types=1);

namespace MageLift\Build\Magento;

use MageLift\Build\Process\ProcessRequest;
use MageLift\Build\Process\ProcessResult;
use MageLift\Build\Process\ProcessRunner;

/**
 * Delegates one upstream ece-patches apply after composer install.
 *
 * Quality Patch IDs are passed only through $_ENV['QUALITY_PATCHES'] as an array
 * so optional patches do not fall back to ambient env or .magento.env.yaml.
 */
final readonly class CloudPatchCommand implements CommandInterface
{
    private const BOOTSTRAP = <<<'PHP'
$ids = json_decode($argv[1] ?? '', true, 512, JSON_THROW_ON_ERROR);
if (!is_array($ids)) {
    fwrite(STDERR, "Invalid quality patch ID payload for Cloud Patches apply.\n");
    exit(1);
}
$_ENV['QUALITY_PATCHES'] = $ids;
$script = getcwd().'/vendor/bin/ece-patches';
if (!is_file($script)) {
    fwrite(STDERR, "Missing vendor/bin/ece-patches after composer install.\n");
    exit(1);
}
$argv = [$script, 'apply'];
$argc = count($argv);
$_SERVER['argv'] = $argv;
$_SERVER['argc'] = $argc;
require $script;
PHP;

    /** @var list<string> */
    private array $qualityPatchIds;

    /** @param list<string> $qualityPatchIds */
    public function __construct(array $qualityPatchIds)
    {
        $this->qualityPatchIds = QualityPatchConfig::normalize($qualityPatchIds);
    }

    public function executable(): Executable
    {
        return Executable::Php;
    }

    public function arguments(): array
    {
        return ['-r', self::BOOTSTRAP, $this->encodedQualityPatchIds()];
    }

    public function argv(): array
    {
        return [Executable::Php->value, '-r', self::BOOTSTRAP, $this->encodedQualityPatchIds()];
    }

    public function run(ProcessRunner $runner, ProcessRequest $request): ProcessResult
    {
        return $runner->run($request);
    }

    private function encodedQualityPatchIds(): string
    {
        return json_encode($this->qualityPatchIds, JSON_THROW_ON_ERROR);
    }
}
