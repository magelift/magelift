#!/usr/bin/env bash
# Write a PHP project that reads only the acceptance harness's generated JSON.
acceptance_project_wrapper() {
    cat > "$1/magelift.php" <<'PHP'
<?php
declare(strict_types=1);
return new \MageLift\Project(json_decode(file_get_contents(__DIR__ . '/acceptance.json'), true, flags: JSON_THROW_ON_ERROR));
PHP
}
