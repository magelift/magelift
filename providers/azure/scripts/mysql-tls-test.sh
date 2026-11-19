#!/usr/bin/env bash
# Account-free integration test: TLS-only MySQL, the shipped PHP bootstrap,
# and both correct and incorrect certificates/hostnames.
set -euo pipefail
root=$(cd "$(dirname "$0")/../../.." && pwd)
work=$(mktemp -d)
network="magelift-azure-tls-$$"
server="magelift-azure-tls-db-$$"
cleanup() { docker rm -f "$server" >/dev/null 2>&1 || true; docker network rm "$network" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=Test CA' -keyout "$work/ca.key" -out "$work/ca.crt" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=Wrong CA' -keyout "$work/wrong.key" -out "$work/wrong.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj '/CN=mysql-tls' -keyout "$work/server.key" -out "$work/server.csr" >/dev/null 2>&1
printf '%s\n' 'subjectAltName=DNS:mysql-tls' 'extendedKeyUsage=serverAuth' > "$work/extensions"
openssl x509 -req -days 1 -in "$work/server.csr" -CA "$work/ca.crt" -CAkey "$work/ca.key" -CAcreateserial -extfile "$work/extensions" -out "$work/server.crt" >/dev/null 2>&1
chmod 755 "$work"
chmod 644 "$work"/*.crt "$work/server.key"
mkdir -p "$work/app/app/etc"
cat > "$work/app/app/etc/env.php" <<'PHP'
<?php return ['db' => ['connection' => ['default' => ['host' => 'mysql-tls', 'dbname' => 'magento', 'username' => 'magento', 'password' => 'integration-password']]]];
PHP
cat > "$work/check.php" <<'PHP'
<?php
require '/source/images/php-nginx/deployment-config.php';
$override = json_decode(getenv('MAGENTO_DC__OVERRIDE'), true, flags: JSON_THROW_ON_ERROR);
$options = $override['db']['connection']['default']['driver_options'];
$caOption = defined('Pdo\\Mysql::ATTR_SSL_CA') ? constant('Pdo\\Mysql::ATTR_SSL_CA') : constant('PDO::MYSQL_ATTR_SSL_CA');
$verifyOption = defined('Pdo\\Mysql::ATTR_SSL_VERIFY_SERVER_CERT') ? constant('Pdo\\Mysql::ATTR_SSL_VERIFY_SERVER_CERT') : constant('PDO::MYSQL_ATTR_SSL_VERIFY_SERVER_CERT');
if (($options[$caOption] ?? null) !== getenv('MAGELIFT_DATABASE_TLS_CA') || ($options[$verifyOption] ?? null) !== true) {
    throw new RuntimeException('Magento PDO TLS verification was not configured');
}
define('MAGELIFT_INITIAL_DATABASE_BOOTSTRAP_LIBRARY', true);
require '/source/build/bin/ensure-initial-database.runtime.php';
$connection = ['host' => getenv('TEST_DB_HOST'), 'dbname' => 'magento', 'username' => 'magento', 'password' => 'integration-password'];
if (getenv('TEST_PHASE') === 'magento') {
    // The same numeric driver options are consumed by Magento's PDO adapter.
    new PDO('mysql:host='.$connection['host'].';dbname=magento', $connection['username'], $connection['password'], $options);
} else {
    magelift_initial_db_count_tables($connection);
}
PHP
docker network create "$network" >/dev/null
docker run -d --name "$server" --network "$network" --network-alias mysql-tls --network-alias wrong-host \
  -e MYSQL_ROOT_PASSWORD=integration-root-password -e MYSQL_DATABASE=magento -e MYSQL_USER=magento -e MYSQL_PASSWORD=integration-password \
  -v "$work:/tls:ro" mysql:8.4.10 --require_secure_transport=ON --ssl-ca=/tls/ca.crt --ssl-cert=/tls/server.crt --ssl-key=/tls/server.key >/dev/null
for i in $(seq 1 90); do
  if docker exec "$server" mysqladmin ping -h127.0.0.1 --ssl-mode=REQUIRED -uroot -pintegration-root-password --silent >/dev/null 2>&1; then break; fi
  if [[ "$i" == 90 ]]; then docker logs "$server" >&2; exit 1; fi
  sleep 1
done
run_php() {
  docker run --rm --network "$network" --entrypoint php -e TEST_DB_HOST="$1" -e MAGELIFT_DATABASE_TLS_CA="$2" -e TEST_PHASE="$3" \
	-v "$work:/tls:ro" -v "$work/app:/app:ro" -v "$root:/source:ro" magelift/php-nginx:local -d auto_prepend_file= /tls/check.php
}
for phase in magento bootstrap; do
  run_php mysql-tls /tls/ca.crt "$phase"
  if run_php mysql-tls /tls/wrong.crt "$phase" >/dev/null 2>&1; then echo "$phase wrong CA unexpectedly succeeded" >&2; exit 1; fi
  if run_php wrong-host /tls/ca.crt "$phase" >/dev/null 2>&1; then echo "$phase wrong hostname unexpectedly succeeded" >&2; exit 1; fi
done
echo 'Azure MySQL TLS: trusted host passed; wrong CA and wrong hostname rejected'
