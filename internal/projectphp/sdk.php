<?php

declare(strict_types=1);

namespace MageLift;

/** A serializable part of a MageLift project definition. */
abstract class Definition implements \JsonSerializable
{
    public function __construct(protected array $values = []) {}

    public function jsonSerialize(): object
    {
        return (object) $this->values;
    }
}

/** The project configuration returned by magelift.php. */
final class Project extends Definition
{
    public function __construct(array $values = [], ?string $name = null, ?Target $target = null, ?array $environments = null, ?array $services = null)
    {
        if ($name !== null) {
            $values['project']['name'] = $name;
        }
        if ($target !== null) {
            $values['target'] = $target;
        }
        if ($environments !== null) {
            $values['environments'] = (object) $environments;
        }
        if ($services !== null) {
            $values['services'] = (object) $services;
        }
        parent::__construct($values);
    }
}

/** Configuration for one deployment environment. */
final class Environment extends Definition
{
    public function __construct(array $values = [], ?string $inherits = null, ?string $class = null, ?string $preset = null, ?string $domain = null)
    {
        foreach (['inherits' => $inherits, 'class' => $class, 'preset' => $preset, 'domain' => $domain] as $key => $value) {
            if ($value !== null) {
                $values[$key] = $value;
            }
        }
        parent::__construct($values);
    }
}

/** Cloud provider and runtime, with provider-specific options in values. */
final class Target extends Definition
{
    public function __construct(array $values = [], ?string $provider = null, ?string $runtime = null)
    {
        if ($provider !== null) {
            $values['provider'] = $provider;
        }
        if ($runtime !== null) {
            $values['runtime'] = $runtime;
        }
        parent::__construct($values);
    }
}

/** Service settings passed to a provider or extension. */
final class Service extends Definition
{
    public function __construct(array $values = [], ?string $family = null, ?string $version = null, ?string $ownership = null, ?string $endpoint = null, ?string $secret = null, ?string $image = null, ?int $port = null, ?string $username = null, ?string $adapter = null, ?string $evictionPolicy = null, ?array $plugins = null, ?int $storageGiB = null, ?int $memoryMiB = null, ?array $parameters = null)
    {
        foreach (compact('family', 'version', 'ownership', 'endpoint', 'secret', 'image', 'port', 'username', 'adapter', 'evictionPolicy', 'plugins', 'storageGiB', 'memoryMiB', 'parameters') as $key => $value) {
            if ($value !== null) {
                $values[$key] = $value;
            }
        }
        parent::__construct($values);
    }
}
