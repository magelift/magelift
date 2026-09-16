#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
env_file="$repo_root/images/php-nginx/env.php"
fpm_pool_file="$repo_root/images/php-nginx/www.conf"

grep -Fq "'user' => '#env(MAGENTO_DC_QUEUE__AMQP__USERNAME, \"magento\")'" "$env_file"
grep -Fq "'password' => '#env(MAGENTO_DC_QUEUE__AMQP__PASSWORD)'" "$env_file"
if grep -Fq "'username' => '#env(MAGENTO_DC_QUEUE__AMQP__USERNAME" "$env_file"; then
  echo "Magento AMQP config must use the 'user' key" >&2
  exit 1
fi

php -r '
$config = require $argv[1];
$installDate = $config["install"]["date"] ?? null;
if ($installDate !== "#env(MAGENTO_DC_INSTALL__DATE, \"Thu, 01 Jan 1970 00:00:00 +0000\")") {
    fwrite(STDERR, "Magento install date must be resolved from the runtime deployment contract\n");
    exit(1);
}
$db = $config["db"]["connection"]["default"] ?? null;
if (!is_array($db) || array_key_exists("port", $db)) {
    fwrite(STDERR, "Magento DB config must not emit a separate port key\n");
    exit(1);
}
$search = $config["system"]["default"]["catalog"]["search"] ?? null;
if (!is_array($search) || ($search["opensearch_server_hostname"] ?? null) !== "#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME, \"127.0.0.1\")") {
    fwrite(STDERR, "Magento search config must be under system/default/catalog/search\n");
    exit(1);
}
if (array_key_exists("catalog", $config)) {
    fwrite(STDERR, "Magento search config must not be emitted as a deployment-config top-level catalog key\n");
    exit(1);
}
' "$env_file"

if ! grep -Eq '^[[:space:]]*clear_env[[:space:]]*=[[:space:]]*no[[:space:]]*$' "$fpm_pool_file"; then
  echo "PHP-FPM must preserve the deployment-config environment" >&2
  exit 1
fi

echo "Magento env contract: pass"
