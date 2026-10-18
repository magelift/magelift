<?php

declare(strict_types=1);

namespace MageLift\Build\Runner;

use MageLift\Build\Protocol\InvalidProtocolRequest;
use MageLift\Build\Protocol\ProtocolJson;
use Throwable;

final readonly class ConsoleApplication
{
    public const EXIT_SUCCESS = 0;
    public const EXIT_INVALID_REQUEST = 2;
    public const EXIT_EXECUTION_FAILED = 3;

    public function __construct(private RunnerService $service)
    {
    }

	/** @param resource $stdin @param resource $stdout @param resource $stderr */
	public function run(mixed $stdin, mixed $stdout, mixed $stderr): int
    {
        $input = stream_get_contents($stdin, ProtocolJson::MAX_BYTES + 1);
        if ($input === false || strlen($input) > ProtocolJson::MAX_BYTES) {
            fwrite($stderr, "invalid request: protocol message exceeds the byte limit\n");

            return self::EXIT_INVALID_REQUEST;
        }
        try {
            fwrite($stdout, $this->service->handle($input));

            return self::EXIT_SUCCESS;
        } catch (InvalidProtocolRequest $error) {
            fwrite($stderr, 'invalid request: '.$error->getMessage()."\n");

            return self::EXIT_INVALID_REQUEST;
        } catch (Throwable $error) {
            fwrite($stderr, 'runner failed: '.$error->getMessage()."\n");

            return self::EXIT_EXECUTION_FAILED;
        }
    }
}
