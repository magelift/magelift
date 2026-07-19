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
    image: ${MAGELIFT_LOCAL_DATABASE_IMAGE:-mysql:8.4@sha256:c592c15aaf4a1961e15d82eb31ea5987dda862d1c4b1e93424438c0e91dc1f8d}
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
    image: ${MAGELIFT_LOCAL_CACHE_IMAGE:-valkey/valkey:8.1@sha256:3e31dd49b6b742e614975e8ab7b1b19809d00ecac7657c6b34bff23582a433cd}
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
    image: ${MAGELIFT_LOCAL_SEARCH_IMAGE:-opensearchproject/opensearch:3@sha256:44ba7ea58a319adf61c33ab16873f9ef5dbb30b291a832d375172f0b2d24e3c9}
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

  queue:
    profiles: ["queue"]
    image: ${MAGELIFT_LOCAL_QUEUE_IMAGE:-rabbitmq:4.2-management@sha256:2f5f2d5551a7c11e09c57b00ff6a86f363ddd424e3bde3a54c88a75c742a81e8}
    environment:
      RABBITMQ_DEFAULT_USER: magento
      RABBITMQ_DEFAULT_PASS: magento
      RABBITMQ_DEFAULT_VHOST: /
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_QUEUE_PORT:-5672}:5672"
      - "127.0.0.1:${MAGELIFT_LOCAL_QUEUE_MANAGEMENT_PORT:-15672}:15672"
    volumes:
      - queue:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 10s
      timeout: 5s
      retries: 30

  app:
    profiles: ["app"]
    image: ${MAGELIFT_LOCAL_APP_IMAGE:-magelift/frankenphp-classic:8.5-local}
    env_file:
      - local.env
    environment:
      MAGENTO_DB_HOST: database
      MAGENTO_DB_NAME: magento
      MAGENTO_DB_USER: magento
      MAGENTO_DB_PASSWORD: magento
      MAGENTO_CACHE_HOST: cache
      MAGENTO_SEARCH_HOST: search
      MAGENTO_QUEUE_HOST: queue
      MAGENTO_QUEUE_USER: magento
      MAGENTO_QUEUE_PASSWORD: magento
    depends_on:
      database:
        condition: service_healthy
      cache:
        condition: service_healthy
      search:
        condition: service_healthy
      queue:
        condition: service_healthy
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_HTTP_PORT:-8080}:8080"
      - "127.0.0.1:${MAGELIFT_LOCAL_HTTPS_PORT:-8443}:8443"
    volumes:
      - type: bind
        source: ${MAGELIFT_PROJECT_ROOT:-.}
        target: /app

volumes:
  database:
  cache:
  search:
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
			args = append(args, "--profile", "app", "--profile", "search", "--profile", "queue")
		case "search", "queue":
			args = append(args, "--profile", service)
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
