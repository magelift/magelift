package localdev

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const ComposeFile = ".magelift/compose.local.yml"

var projectNamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

const ComposeTemplate = `services:
  database:
    image: ${MAGELIFT_LOCAL_DATABASE_IMAGE:-mysql:8.4@sha256:b3b90af2a6552ae30c266fdb7d5dd55f3afb72404bb78d37fe8a23eb857fd3fb}
    environment:
      MYSQL_DATABASE: magento
      MYSQL_USER: magento
      MYSQL_PASSWORD: magento
      MYSQL_ROOT_PASSWORD: root
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_DATABASE_PORT:-3306}:3306"
    volumes:
      - database:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -uroot -proot"]
      interval: 5s
      timeout: 3s
      retries: 20

  cache:
    image: ${MAGELIFT_LOCAL_CACHE_IMAGE:-valkey/valkey:9@sha256:3acc0687f2a2e1091fae6450d7842dd658c941338cf0a873ddd9e14b9e4ea4dd}
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_CACHE_PORT:-6379}:6379"
    volumes:
      - cache:/data
    healthcheck:
      test: ["CMD", "valkey-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  search:
    profiles: ["search"]
    image: ${MAGELIFT_LOCAL_SEARCH_IMAGE:-opensearchproject/opensearch:3@sha256:bcc1797519726ceb6d651d4a3e60b7c30da91793914a8dfe75fd441d4f641509}
    environment:
      discovery.type: single-node
      DISABLE_SECURITY_PLUGIN: "true"
      OPENSEARCH_JAVA_OPTS: -Xms512m -Xmx512m
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_SEARCH_PORT:-9200}:9200"
    volumes:
      - search:/usr/share/opensearch/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:9200/_cluster/health"]
      interval: 10s
      timeout: 5s
      retries: 30

__MAGELIFT_QUEUE_SERVICE__
__MAGELIFT_SESSION_SERVICE__
__MAGELIFT_MAILPIT_SERVICE__
  app:
    profiles: ["app"]
    image: ${MAGELIFT_LOCAL_APP_IMAGE:-magelift/php-nginx:8.5-local}
    env_file:
      - local.env
    environment:
      MAGELIFT_LOCAL_MAGENTO_VERSION: "2.4.9"
      MAGELIFT_LOCAL_PHP: "8.5"
      MAGELIFT_LOCAL_COMPOSER: "2.10"
      MAGELIFT_LOCAL_EXTENSIONS: ""
      MAGELIFT_LOCAL_PHP_SETTINGS: ""
      MAGELIFT_LOCAL_DATABASE_FAMILY: "mysql"
      MAGELIFT_LOCAL_DATABASE_VERSION: "8.4"
      MAGELIFT_LOCAL_CACHE_FAMILY: "valkey"
      MAGELIFT_LOCAL_CACHE_VERSION: "9"
      MAGELIFT_LOCAL_SEARCH_FAMILY: "opensearch"
      MAGELIFT_LOCAL_SEARCH_VERSION: "3"
      MAGELIFT_LOCAL_QUEUE_FAMILY: "rabbitmq"
      MAGELIFT_LOCAL_QUEUE_VERSION: "4.2"
      MAGELIFT_LOCAL_WEB_SERVER_FAMILY: "nginx"
      MAGELIFT_LOCAL_WEB_SERVER_VERSION: "8.5"
      MAGELIFT_LOCAL_WEB_CACHE_FAMILY: "none"
      MAGELIFT_LOCAL_WEB_CACHE_VERSION: ""
      MAGELIFT_LOCAL_EMAIL_MODE: "disabled"
      MAGELIFT_LOCAL_EMAIL_HOST: ""
      MAGELIFT_LOCAL_EMAIL_PORT: "0"
      MAGELIFT_LOCAL_EMAIL_USERNAME: ""
      MAGELIFT_LOCAL_EMAIL_FROM: ""
      MAGELIFT_LOCAL_EMAIL_CREDENTIAL_ENV: ""
      MAGELIFT_LOCAL_EMAIL_DISABLE: "1"
      MAGELIFT_LOCAL_EMAIL_TRANSPORT: "smtp"
      MAGELIFT_LOCAL_EMAIL_AUTH: "NONE"
      MAGELIFT_LOCAL_EMAIL_SSL: ""
      MAGELIFT_LOCAL_EMAIL_SET_RETURN_PATH: "2"
      MAGELIFT_LOCAL_EMAIL_RETURN_PATH_EMAIL: ""
      MAGENTO_DC__OVERRIDE: ""
      MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST: database
      MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT: "3306"
      MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME: magento
      MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME: magento
      MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD: magento
      MAGENTO_DC_DB__CONNECTION__DEFAULT__MODEL: mysql4
      MAGENTO_DC_DB__CONNECTION__DEFAULT__ENGINE: innodb
      MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__SERVER: cache
      MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__PORT: "6379"
      MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__SERVER: cache
      MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__PORT: "6379"
      MAGENTO_DC_SESSION__SAVE: files
      MAGENTO_DC_SESSION__REDIS_HOST: ""
      MAGENTO_DC_SESSION__REDIS_PORT: "0"
      MAGENTO_DC_CATALOG__SEARCH__ENGINE: opensearch
      MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME: search
      MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT: "9200"
      MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH: "0"
      CONFIG__DEFAULT__CATALOG__SEARCH__ENGINE: opensearch
      CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME: search
      CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_PORT: "9200"
      CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH: "0"
      MAGENTO_DC_QUEUE__DEFAULT_CONNECTION: amqp
      MAGENTO_DC_QUEUE__AMQP__HOST: queue
      MAGENTO_DC_QUEUE__AMQP__PORT: "5672"
      MAGENTO_DC_QUEUE__AMQP__SSL: "0"
      MAGENTO_DC_QUEUE__AMQP__USERNAME: magento
      MAGENTO_DC_QUEUE__AMQP__PASSWORD: magento
      MAGENTO_DC_QUEUE__STOMP__HOST: queue
      MAGENTO_DC_QUEUE__STOMP__PORT: "61613"
      MAGENTO_DC_QUEUE__STOMP__SSL: "0"
      MAGENTO_DC_QUEUE__STOMP__USER: magento
      MAGENTO_DC_QUEUE__STOMP__PASSWORD: magento
    command: ["php-fpm", "--nodaemonize"]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:8080/health"]
      interval: 5s
      timeout: 3s
      retries: 20
    depends_on:
      database:
        condition: service_healthy
      cache:
        condition: service_healthy
__MAGELIFT_SESSION_DEPENDS__      search:
        condition: service_healthy
      queue:
        condition: service_healthy
__MAGELIFT_MAILPIT_DEPENDS__    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_APP_HTTP_PORT:-8080}:8080"
      - "127.0.0.1:${MAGELIFT_LOCAL_HTTPS_PORT:-8443}:8443"
    volumes:
      - type: bind
        source: ${MAGELIFT_PROJECT_ROOT:-.}
        target: /app
      - type: bind
        source: ${MAGELIFT_PROJECT_ROOT:-.}/.magelift/local.php.ini
        target: /usr/local/etc/php/conf.d/zz-magelift-local.ini
        read_only: true

__MAGELIFT_VARNISH_SERVICE__

volumes:
  database:
  cache:
__MAGELIFT_SESSION_VOLUME__  search:
  queue:
`

func ProjectName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	name = projectNamePattern.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "", errors.New("project name is required")
	}
	if len(name) > 48 {
		name = strings.Trim(name[:48], "-")
	}
	return "magelift-" + name, nil
}

func ComposeArgs(file, project, action, service string, command []string) ([]string, error) {
	file = filepath.Clean(strings.TrimSpace(file))
	project = strings.TrimSpace(project)
	if file == "." || file == "" {
		return nil, errors.New("local compose file is required")
	}
	if project == "" {
		return nil, errors.New("local compose project name is required")
	}
	args := []string{"compose", "-f", file, "--project-name", project}
	switch action {
	case "up":
		switch service {
		case "app":
			args = append(args, "--profile", "app", "--profile", "search", "--profile", "queue", "--profile", "web-cache", "--profile", "email")
		case "search", "queue", "varnish":
			profile := service
			if service == "varnish" {
				profile = "web-cache"
			}
			args = append(args, "--profile", profile)
		}
		args = append(args, "up", "-d")
	case "down":
		args = append(args, "down", "--remove-orphans")
	case "reset":
		args = append(args, "down", "--volumes", "--remove-orphans")
	case "status":
		args = append(args, "ps")
	case "logs":
		args = append(args, "logs", "--no-color", "--tail=200")
	case "exec":
		service = strings.TrimSpace(service)
		if service == "" {
			return nil, errors.New("local service is required for exec")
		}
		if len(command) == 0 {
			return nil, errors.New("a command is required for local exec")
		}
		args = append(args, "exec", service)
		args = append(args, command...)
	default:
		return nil, fmt.Errorf("unsupported local action %q", action)
	}
	if service != "" && action != "exec" {
		args = append(args, service)
	}
	return args, nil
}

// LocalEnvFile is stored beside the generated Compose file and is deliberately
// separate from the project configuration. It contains local-only credentials.
const LocalEnvFile = "local.env"

const LocalPHPIniFile = "local.php.ini"
