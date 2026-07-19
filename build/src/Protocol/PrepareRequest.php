<?php

declare(strict_types=1);

namespace MageLift\Build\Protocol;

final readonly class PrepareRequest
{
    /**
     * @param array{edition: string, version: string, mode: string, webRuntime: string} $application
     * @param list<array{path: string, sha256: string}> $inputFiles
     * @param list<array{locale: string, theme: string}> $staticContent
     * @param list<array<string, mixed>> $lifecycleHooks
     */
    private function __construct(
        public string $repositoryRoot,
        public string $sourceRevision,
        public array $application,
        public string $phpVersion,
        public string $compatibilityStatus,
        public array $inputFiles,
        public array $staticContent,
        public array $lifecycleHooks,
    ) {
    }

    public static function fromJson(string $json): self
    {
        $envelope = ProtocolJson::decodeObject($json);
        ProtocolJson::assertKeys($envelope, ['protocolVersion', 'stage', 'prepare'], ['protocolVersion', 'stage', 'prepare']);
        ProtocolJson::assertEnvelope($envelope, ProtocolOperation::Prepare);
        $payload = ProtocolJson::object($envelope['prepare'], 'prepare');
        ProtocolJson::assertKeys(
            $payload,
            ['repositoryRoot', 'sourceRevision', 'application', 'phpVersion', 'compatibilityStatus', 'inputFiles', 'staticContent', 'lifecycleHooks'],
            ['repositoryRoot', 'sourceRevision', 'application', 'phpVersion', 'compatibilityStatus', 'inputFiles', 'staticContent'],
            'prepare',
        );
        $application = ProtocolJson::object($payload['application'], 'prepare.application');
        ProtocolJson::assertKeys(
            $application,
            ['edition', 'version', 'mode', 'webRuntime'],
            ['edition', 'version', 'mode', 'webRuntime'],
            'prepare.application',
        );
        try {
            ProtocolJson::assertNoSecrets($payload);
        } catch (ProtocolException $error) {
            throw new InvalidProtocolRequest('Prepare payload contains a forbidden secret.', previous: $error);
        }
        self::validateApplication($application);
        $application = [
            'edition' => $application['edition'],
            'version' => $application['version'],
            'mode' => $application['mode'],
            'webRuntime' => $application['webRuntime'],
        ];

        $repositoryRoot = ProtocolJson::string($payload['repositoryRoot'], 'prepare.repositoryRoot');
        if (!str_starts_with($repositoryRoot, '/')) {
            throw new InvalidProtocolRequest('repositoryRoot must be an absolute path.');
        }
        $sourceRevision = ProtocolJson::revision($payload['sourceRevision']);
        $phpVersion = self::requiredString($payload['phpVersion'], 'prepare.phpVersion');
        $compatibilityStatus = ProtocolJson::string($payload['compatibilityStatus'], 'prepare.compatibilityStatus');
        if (!in_array($compatibilityStatus, ['supported', 'unsupported-allowed'], true)) {
            throw new InvalidProtocolRequest('compatibilityStatus must be supported or unsupported-allowed.');
        }
        $inputFiles = self::parseInputFiles($payload['inputFiles']);
        $staticContent = self::parseStaticContent($payload['staticContent']);
        $lifecycleHooks = self::parseLifecycleHooks($payload['lifecycleHooks'] ?? []);
        return new self($repositoryRoot, $sourceRevision, $application, $phpVersion, $compatibilityStatus, $inputFiles, $staticContent, $lifecycleHooks);
    }

    public function toCanonicalJson(): string
    {
        $inputFiles = $this->inputFiles;
        usort($inputFiles, static fn (array $a, array $b): int => strcmp($a['path'], $b['path']));
        $staticContent = $this->staticContent;
        usort($staticContent, static fn (array $a, array $b): int => [$a['locale'], $a['theme']] <=> [$b['locale'], $b['theme']]);

        $prepare = [
            'repositoryRoot' => $this->repositoryRoot,
            'sourceRevision' => $this->sourceRevision,
            'application' => $this->application,
            'phpVersion' => $this->phpVersion,
            'compatibilityStatus' => $this->compatibilityStatus,
            'inputFiles' => $inputFiles,
            'staticContent' => $staticContent,
        ];
        if ($this->lifecycleHooks !== []) {
            $prepare['lifecycleHooks'] = array_map(static function (array $hook): array {
                $canonical = [
                    'id' => $hook['id'],
                    'phase' => $hook['phase'],
                    'relationship' => $hook['relationship'],
                    'target' => $hook['target'],
                ];
                if ($hook['command'] !== null) {
                    $canonical['command'] = ['executable' => $hook['command']['executable']];
                    if ($hook['command']['arguments'] !== []) {
                        $canonical['command']['arguments'] = $hook['command']['arguments'];
                    }
                }
                if ($hook['dependencies'] !== []) {
                    $canonical['dependencies'] = $hook['dependencies'];
                }
                $canonical['timeoutSeconds'] = $hook['timeoutSeconds'];
                $canonical['retries'] = $hook['retries'];
                $canonical['failure'] = $hook['failure'];

                return $canonical;
            }, $this->lifecycleHooks);
        }

        return ProtocolJson::encode([
            'protocolVersion' => ProtocolJson::VERSION,
            'stage' => ProtocolOperation::Prepare->value,
            'prepare' => $prepare,
        ]);
    }

    /** @param array<string, mixed> $application */
    private static function validateApplication(array $application): void
    {
        $edition = ProtocolJson::string($application['edition'], 'prepare.application.edition');
        if (!in_array($edition, ['open-source', 'commerce'], true)) {
            throw new InvalidProtocolRequest('application edition must be open-source or commerce.');
        }
        self::requiredString($application['version'], 'prepare.application.version');
        $mode = ProtocolJson::string($application['mode'], 'prepare.application.mode');
        if (!in_array($mode, ['integrated', 'headless'], true)) {
            throw new InvalidProtocolRequest('application mode must be integrated or headless.');
        }
        self::requiredString($application['webRuntime'], 'prepare.application.webRuntime');
    }

    private static function requiredString(mixed $value, string $field): string
    {
        $string = ProtocolJson::string($value, $field);
        if ($string === '') {
            throw new InvalidProtocolRequest(sprintf('%s is required.', $field));
        }

        return $string;
    }

    /** @return list<array{path: string, sha256: string}> */
    private static function parseInputFiles(mixed $value): array
    {
        $entries = ProtocolJson::list($value, 'prepare.inputFiles');
        if ($entries === []) {
            throw new InvalidProtocolRequest('At least one immutable build input file is required.');
        }
        $result = [];
        $paths = [];
        foreach ($entries as $entry) {
            $input = ProtocolJson::object($entry, 'prepare.inputFiles[]');
            ProtocolJson::assertKeys($input, ['path', 'sha256'], ['path', 'sha256'], 'prepare.inputFiles[]');
            $path = ProtocolJson::relativePath($input['path'], 'build input');
            if (isset($paths[$path])) {
                throw new InvalidProtocolRequest('Duplicate build input path.');
            }
            $paths[$path] = true;
            $result[] = ['path' => $path, 'sha256' => ProtocolJson::checksum($input['sha256'], 'build input')];
        }

        return $result;
    }

    /** @return list<array{locale: string, theme: string}> */
    private static function parseStaticContent(mixed $value): array
    {
        $result = [];
        $seen = [];
        foreach (ProtocolJson::list($value, 'prepare.staticContent') as $entry) {
            $content = ProtocolJson::object($entry, 'prepare.staticContent[]');
            ProtocolJson::assertKeys($content, ['locale', 'theme'], ['locale', 'theme'], 'prepare.staticContent[]');
            $locale = ProtocolJson::nonEmptyString($content['locale'], 'static content locale');
            $theme = ProtocolJson::nonEmptyString($content['theme'], 'static content theme');
            $key = $locale."\0".$theme;
            if (isset($seen[$key])) {
                throw new InvalidProtocolRequest('Duplicate static content entry.');
            }
            $seen[$key] = true;
            $result[] = ['locale' => $locale, 'theme' => $theme];
        }

        return $result;
    }

    /** @return list<array<string, mixed>> */
    private static function parseLifecycleHooks(mixed $value): array
    {
        $result = [];
        $ids = [];
        foreach (ProtocolJson::list($value, 'prepare.lifecycleHooks') as $entry) {
            $hook = ProtocolJson::object($entry, 'prepare.lifecycleHooks[]');
            ProtocolJson::assertKeys(
                $hook,
                ['id', 'phase', 'relationship', 'target', 'command', 'dependencies', 'timeoutSeconds', 'retries', 'failure'],
                ['id', 'phase', 'relationship', 'target'],
                'prepare.lifecycleHooks[]',
            );
            $id = self::stableId($hook['id'], 'lifecycle hook ID');
            if (isset($ids[$id])) {
                throw new InvalidProtocolRequest(sprintf('Duplicate lifecycle hook ID "%s".', $id));
            }
            $ids[$id] = true;
            $phase = ProtocolJson::string($hook['phase'], 'lifecycle hook phase');
            if (!in_array($phase, ['validate', 'build', 'package'], true)) {
                throw new InvalidProtocolRequest('Lifecycle hook phase must be validate, build, or package.');
            }
            $relationship = ProtocolJson::string($hook['relationship'], 'lifecycle hook relationship');
            if (!in_array($relationship, ['before', 'after', 'replace', 'disable'], true)) {
                throw new InvalidProtocolRequest('Lifecycle hook relationship is invalid.');
            }
            $target = self::stableId($hook['target'], 'lifecycle hook target');
            $command = null;
            if (array_key_exists('command', $hook) && $hook['command'] !== null) {
                $commandValue = ProtocolJson::object($hook['command'], 'lifecycle hook command');
                ProtocolJson::assertKeys($commandValue, ['executable', 'arguments'], ['executable'], 'lifecycle hook command');
                $executable = ProtocolJson::string($commandValue['executable'], 'lifecycle hook executable');
                if (!in_array($executable, ['composer', 'magento'], true)) {
                    throw new InvalidProtocolRequest('Lifecycle hook executable must be composer or magento.');
                }
                $arguments = [];
                foreach (ProtocolJson::list($commandValue['arguments'] ?? [], 'lifecycle hook arguments') as $argument) {
                    $arguments[] = ProtocolJson::nonEmptyString($argument, 'lifecycle hook argument');
                }
                $command = ['executable' => $executable, 'arguments' => $arguments];
            }
            if ($relationship === 'disable' && $command !== null) {
                throw new InvalidProtocolRequest('A disabled lifecycle hook cannot define a command.');
            }
            if ($relationship !== 'disable' && $command === null) {
                throw new InvalidProtocolRequest('A lifecycle hook command is required unless it is disabled.');
            }
            $dependencies = [];
            foreach (ProtocolJson::list($hook['dependencies'] ?? [], 'lifecycle hook dependencies') as $dependency) {
                $dependencies[] = self::stableId($dependency, 'lifecycle hook dependency');
            }
            $dependencies = array_values(array_unique($dependencies));
            sort($dependencies, SORT_STRING);
            $timeout = self::optionalInt($hook['timeoutSeconds'] ?? null, 300, 'lifecycle hook timeoutSeconds');
            if ($timeout < 1 || $timeout > 7200) {
                throw new InvalidProtocolRequest('Lifecycle hook timeoutSeconds must be between 1 and 7200.');
            }
            $retriesValue = array_key_exists('retries', $hook)
                ? ProtocolJson::object($hook['retries'], 'lifecycle hook retries')
                : [];
            ProtocolJson::assertKeys($retriesValue, ['maxAttempts', 'delaySeconds', 'idempotent'], [], 'lifecycle hook retries');
            $maxAttempts = self::optionalInt($retriesValue['maxAttempts'] ?? null, 1, 'lifecycle hook maxAttempts');
            $delaySeconds = self::optionalInt($retriesValue['delaySeconds'] ?? null, 0, 'lifecycle hook delaySeconds');
            $idempotent = $retriesValue['idempotent'] ?? false;
            if (!is_bool($idempotent)) {
                throw new InvalidProtocolRequest('Lifecycle hook idempotent must be a boolean.');
            }
            if ($maxAttempts < 1 || $maxAttempts > 5 || $delaySeconds < 0 || $delaySeconds > 3600 || $maxAttempts > 1 && !$idempotent) {
                throw new InvalidProtocolRequest('Lifecycle hook retry policy is invalid.');
            }
            $failure = $hook['failure'] ?? 'abort';
            $failure = ProtocolJson::string($failure, 'lifecycle hook failure');
            if (!in_array($failure, ['abort', 'continue'], true)) {
                throw new InvalidProtocolRequest('Lifecycle hook failure must be abort or continue.');
            }
            $result[] = [
                'id' => $id,
                'phase' => $phase,
                'relationship' => $relationship,
                'target' => $target,
                'command' => $command,
                'dependencies' => $dependencies,
                'timeoutSeconds' => $timeout,
                'retries' => ['maxAttempts' => $maxAttempts, 'delaySeconds' => $delaySeconds, 'idempotent' => $idempotent],
                'failure' => $failure,
            ];
        }
        usort($result, static fn (array $a, array $b): int => strcmp($a['id'], $b['id']));

        return $result;
    }

    private static function stableId(mixed $value, string $field): string
    {
        $id = ProtocolJson::nonEmptyString($value, $field);
        if (preg_match('/^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$/D', $id) !== 1) {
            throw new InvalidProtocolRequest(sprintf('%s must be a stable lifecycle ID.', $field));
        }

        return $id;
    }

    private static function optionalInt(mixed $value, int $default, string $field): int
    {
        if ($value === null) {
            return $default;
        }
        if (!is_int($value)) {
            throw new InvalidProtocolRequest(sprintf('%s must be an integer.', $field));
        }

        return $value;
    }
}
