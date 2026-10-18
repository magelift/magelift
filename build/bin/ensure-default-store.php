#!/usr/bin/env php
<?php

declare(strict_types=1);

use MageLift\Build\Magento\DefaultStoreScaffold;

require dirname(__DIR__).'/vendor/autoload.php';

try {
    (new DefaultStoreScaffold())->ensure(getcwd().'/app/etc/config.php');
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage()."\n");
    exit(1);
}
