package localdev

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

const (
	localCatalogUpdatedAt = "2026-08-15"
	localCatalogSource    = "https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements?lang=en"

	localMySQL84Image     = "mysql:8.4"
	localMariaDB1011Image = "mariadb:10.11"
	localMariaDB118Image  = "mariadb:11.8"
	localValkey81Image    = "valkey/valkey:8.1"
	localValkey9Image     = "valkey/valkey:9"
	localOpenSearch2Image = "opensearchproject/opensearch:2"
	localOpenSearch3Image = "opensearchproject/opensearch:3.9.0"
	localRabbitMQ43Image  = "rabbitmq:4.3-management"
	localRedis72Image     = "redis:7.2"
	localArtemis238Image  = "apache/activemq-artemis:2.38.0-alpine"
	localVarnish8Image    = "varnish:8"
	localMailpitImage     = "axllent/mailpit:v1.34.0@sha256:23a2ef85bc50853ab3093645d1d64679afd8887e89da5e48a88940c172591a4e"
)

var localPHPSettingName = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
var localCredentialEnvName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var localSupportedExtensions = map[string]struct{}{
	"apcu": {}, "bcmath": {}, "curl": {}, "dom": {}, "fileinfo": {},
	"ftp": {}, "gd": {}, "iconv": {}, "intl": {}, "json": {}, "mbstring": {},
	"mysqlnd": {}, "opcache": {}, "openssl": {}, "pdo": {}, "pdo_mysql": {},
	"pdo_sqlite": {}, "redis": {}, "soap": {}, "sockets": {}, "sodium": {},
	"xml": {}, "xmlreader": {}, "xmlwriter": {}, "xsl": {}, "zip": {},
}

type LocalServicePlan struct {
	Component   string                   `json:"component" yaml:"component"`
	Family      string                   `json:"family" yaml:"family"`
	Version     string                   `json:"version" yaml:"version"`
	Image       string                   `json:"image" yaml:"image"`
	Ports       []int                    `json:"ports" yaml:"ports"`
	Healthcheck string                   `json:"healthcheck" yaml:"healthcheck"`
	Credentials []LocalCredentialBinding `json:"credentials" yaml:"credentials"`
	Connection  LocalConnectionShape     `json:"connection" yaml:"connection"`
}

// LocalCredentialBinding describes the environment variable used to wire a
// local credential without including its value in a runtime plan.
type LocalCredentialBinding struct {
	Name        string `json:"name" yaml:"name"`
	Environment string `json:"environment" yaml:"environment"`
}

// LocalConnectionShape is the non-secret connection contract consumed by the
// generated Magento environment.
type LocalConnectionShape struct {
	Host           string `json:"host" yaml:"host"`
	Port           int    `json:"port" yaml:"port"`
	ManagementPort int    `json:"managementPort,omitempty" yaml:"managementPort,omitempty"`
	Database       string `json:"database,omitempty" yaml:"database,omitempty"`
	User           string `json:"user,omitempty" yaml:"user,omitempty"`
	PasswordEnv    string `json:"passwordEnvironment,omitempty" yaml:"passwordEnvironment,omitempty"`
	VHost          string `json:"vhost,omitempty" yaml:"vhost,omitempty"`
	Scheme         string `json:"scheme,omitempty" yaml:"scheme,omitempty"`
}

type RuntimePlan struct {
	MagentoVersion   string                    `json:"magentoVersion" yaml:"magentoVersion"`
	PHP              string                    `json:"php" yaml:"php"`
	Composer         string                    `json:"composer" yaml:"composer"`
	Extensions       []string                  `json:"extensions" yaml:"extensions"`
	PHPSettings      map[string]string         `json:"phpSettings" yaml:"phpSettings"`
	Email            config.LocalEmailSettings `json:"email" yaml:"email"`
	AppImage         string                    `json:"appImage" yaml:"appImage"`
	Database         LocalServicePlan          `json:"database" yaml:"database"`
	Cache            LocalServicePlan          `json:"cache" yaml:"cache"`
	Search           LocalServicePlan          `json:"search" yaml:"search"`
	Queue            LocalServicePlan          `json:"queue" yaml:"queue"`
	WebServer        LocalServicePlan          `json:"webServer" yaml:"webServer"`
	WebCache         LocalServicePlan          `json:"webCache" yaml:"webCache"`
	Session          LocalServicePlan          `json:"session,omitempty" yaml:"session,omitempty"`
	Environment      string                    `json:"environment,omitempty" yaml:"environment,omitempty"`
	Substitutes      []Substitute              `json:"substitutes,omitempty" yaml:"substitutes,omitempty"`
	CatalogUpdatedAt string                    `json:"catalogUpdatedAt" yaml:"catalogUpdatedAt"`
	CatalogSource    string                    `json:"catalogSource" yaml:"catalogSource"`
	Warnings         []string                  `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Magento          platform.MagentoOverlays  `json:"magento,omitempty" yaml:"magento,omitempty"`
}

type localReleaseRow struct {
	Release     string
	PHPBranches []string
	Composer    string
	Database    []localServiceRow
	Cache       []localServiceRow
	Search      []localServiceRow
	Queue       []localServiceRow
	WebServer   []localServiceRow
	WebCache    []localServiceRow
}

type localServiceRow struct {
	Family      string
	Version     string
	Image       string
	Ports       []int
	Healthcheck string
	Credentials []LocalCredentialBinding
	Connection  LocalConnectionShape
}

// Default service families are independent of a Magento compatibility matrix.
func defaultServices() localReleaseRow {
	return localReleaseRow{
		Release: "2.4.9", PHPBranches: []string{"8.5"}, Composer: "2.10",
		Database:  databaseRows(localServiceRow{Family: "mysql", Version: "8.4", Image: localMySQL84Image}, localServiceRow{Family: "mariadb", Version: "11.8", Image: localMariaDB118Image}),
		Cache:     cacheRows(localServiceRow{Family: "valkey", Version: "9", Image: localValkey9Image}, localServiceRow{Family: "redis", Version: "7.2", Image: localRedis72Image}),
		Search:    searchRows(localServiceRow{Family: "opensearch", Version: "3", Image: localOpenSearch3Image}),
		Queue:     queueRows(localServiceRow{Family: "rabbitmq", Version: "4.3", Image: localRabbitMQ43Image}, localServiceRow{Family: "artemis", Version: "2", Image: localArtemis238Image}),
		WebServer: webServerRows(localServiceRow{Family: "nginx", Version: "1.30"}),
		WebCache:  webCacheRows(localServiceRow{Family: "none"}, localServiceRow{Family: "varnish", Version: "8", Image: localVarnish8Image}),
	}
}

func databaseRows(rows ...localServiceRow) []localServiceRow {
	return serviceRows("database", []int{3306}, "mysqladmin ping -h 127.0.0.1 -uroot -proot", rows...)
}

func cacheRows(rows ...localServiceRow) []localServiceRow {
	for index := range rows {
		healthcheck := "valkey-cli ping"
		if rows[index].Family == "redis" {
			healthcheck = "redis-cli ping"
		}
		serviceRowsFor(&rows[index], "cache", []int{6379}, healthcheck)
	}
	return rows
}

func searchRows(rows ...localServiceRow) []localServiceRow {
	return serviceRows("search", []int{9200}, "curl -fsS http://127.0.0.1:9200/_cluster/health", rows...)
}

func queueRows(rows ...localServiceRow) []localServiceRow {
	for index := range rows {
		ports := []int{5672, 15672}
		healthcheck := "rabbitmq-diagnostics -q ping"
		if rows[index].Family == "artemis" {
			ports = []int{61613, 8161}
			healthcheck = "nc -w 1 127.0.0.1 61613 </dev/null && nc -w 1 127.0.0.1 8161 </dev/null"
		}
		serviceRowsFor(&rows[index], "queue", ports, healthcheck)
	}
	return rows
}

func serviceRows(component string, ports []int, healthcheck string, rows ...localServiceRow) []localServiceRow {
	for index := range rows {
		serviceRowsFor(&rows[index], component, ports, healthcheck)
	}
	return rows
}

func serviceRowsFor(row *localServiceRow, component string, ports []int, healthcheck string) {
	row.Ports = append([]int(nil), ports...)
	row.Healthcheck = healthcheck
	row.Credentials, row.Connection = serviceContract(component, row.Family, ports)
}

func webServerRows(rows ...localServiceRow) []localServiceRow {
	return serviceRows("web-server", []int{8080}, "curl -fsS http://127.0.0.1:8080/health", rows...)
}

func webCacheRows(rows ...localServiceRow) []localServiceRow {
	for index := range rows {
		if rows[index].Family == "none" {
			rows[index].Credentials = nil
			rows[index].Connection = LocalConnectionShape{Host: "app", Port: 8080}
			continue
		}
		serviceRowsFor(&rows[index], "web-cache", []int{80}, "varnishadm ping")
	}
	return rows
}

func serviceContract(component, family string, ports []int) ([]LocalCredentialBinding, LocalConnectionShape) {
	switch component {
	case "database":
		return []LocalCredentialBinding{
			{Name: "application-user", Environment: "MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME"},
			{Name: "application-password", Environment: "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD"},
		}, LocalConnectionShape{
			Host: "database", Port: 3306, Database: "magento", User: "magento", PasswordEnv: "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD",
		}
	case "cache":
		return nil, LocalConnectionShape{Host: "cache", Port: 6379}
	case "search":
		return nil, LocalConnectionShape{Host: "search", Port: 9200, Scheme: "http"}
	case "queue":
		if family == "artemis" {
			return []LocalCredentialBinding{
				{Name: "application-user", Environment: "MAGENTO_DC_QUEUE__STOMP__USER"},
				{Name: "application-password", Environment: "MAGENTO_DC_QUEUE__STOMP__PASSWORD"},
			}, LocalConnectionShape{
				Host: "queue", Port: 61613, ManagementPort: 8161, User: "magento", PasswordEnv: "MAGENTO_DC_QUEUE__STOMP__PASSWORD", Scheme: "stomp",
			}
		}
		managementPort := 15672
		if len(ports) > 1 {
			managementPort = ports[1]
		}
		return []LocalCredentialBinding{
			{Name: "application-user", Environment: "MAGENTO_DC_QUEUE__AMQP__USERNAME"},
			{Name: "application-password", Environment: "MAGENTO_DC_QUEUE__AMQP__PASSWORD"},
		}, LocalConnectionShape{
			Host: "queue", Port: 5672, ManagementPort: managementPort, User: "magento", PasswordEnv: "MAGENTO_DC_QUEUE__AMQP__PASSWORD", VHost: "/",
		}
	default:
		connection := LocalConnectionShape{Host: component}
		if len(ports) > 0 {
			connection.Port = ports[0]
		}
		return nil, connection
	}
}

// Plan resolves a local runtime before Compose is written. The catalog is
// intentionally smaller than the provider matrix: unsupported local service
// families fail with their nearest verified choice instead of being silently
// replaced.
func Plan(build config.BuildSpec) (RuntimePlan, error) {
	return PlanFor(build, CloudHints{})
}

// PlanFor resolves local Compose against the selected cloud environment.
func PlanFor(build config.BuildSpec, hints CloudHints) (RuntimePlan, error) {
	row, ok := localReleaseRowFor(build.Application.Version)
	if !ok {
		row = defaultServices()
	}

	php := strings.TrimSpace(build.Build.PHP)
	if php == "" {
		return RuntimePlan{}, fmt.Errorf("no local app image exists for PHP %q", php)
	}
	composer := strings.TrimSpace(build.Build.Composer.Version)

	local := build.Local
	if err := applyCloudHints(&local, hints, row); err != nil {
		return RuntimePlan{}, err
	}
	database, err := chooseService("database", local.Database, row.Database)
	if err != nil {
		return RuntimePlan{}, err
	}
	cache, unsupportedCache, err := chooseCache(local.Cache, row.Cache)
	if err != nil {
		return RuntimePlan{}, err
	}
	search, err := chooseService("search", local.Search, row.Search)
	if err != nil {
		return RuntimePlan{}, err
	}
	queue, err := chooseService("queue", local.Queue, row.Queue)
	if err != nil {
		return RuntimePlan{}, err
	}
	webServer, err := chooseWebServer(local.WebServer, build.Application.WebRuntime, php)
	if err != nil {
		return RuntimePlan{}, err
	}
	webCache, err := chooseService("web-cache", local.WebCache, row.WebCache)
	if err != nil {
		return RuntimePlan{}, err
	}
	email, emailWarnings, err := normalizeEmailSettings(local.Email)
	if err != nil {
		return RuntimePlan{}, err
	}

	settings := make(map[string]string, len(local.PHPSettings))
	for key, value := range local.PHPSettings {
		settings[key] = value
	}
	extensions := append([]string(nil), build.Build.Extensions...)
	for _, extension := range extensions {
		if _, ok := localSupportedExtensions[extension]; !ok && local.WebServer.Image == "" {
			return RuntimePlan{}, fmt.Errorf("local PHP extension %q has no verified app-image contract; supported local extensions are %s", extension, strings.Join(sortedLocalExtensions(), ", "))
		}
	}
	sort.Strings(extensions)
	appImage := webServer.Image
	plan := RuntimePlan{
		MagentoVersion:   build.Application.Version,
		PHP:              php,
		Composer:         composer,
		Extensions:       extensions,
		PHPSettings:      settings,
		Email:            email,
		AppImage:         appImage,
		Database:         database,
		Cache:            cache,
		Search:           search,
		Queue:            queue,
		WebServer:        webServer,
		WebCache:         webCache,
		Environment:      hints.Environment,
		CatalogUpdatedAt: localCatalogUpdatedAt,
		CatalogSource:    localCatalogSource,
		Magento:          platform.NewMagentoOverlays(build.Application.Magento.FrontName, build.Application.Magento.CookieDomain, build.Application.Magento.UnsecureBaseURL, build.Application.Magento.SecureBaseURL, build.Application.Magento.StorefrontOrigin, build.Application.Magento.Consumers.Mode, build.Application.Magento.CORSOrigins, build.Application.Magento.Consumers.Names, build.Application.Magento.Variables),
	}
	if hints.IsolateCacheAndSession {
		plan.Session = sessionPlanFromCache(cache)
	}
	plan.Substitutes = namedSubstitutes(hints, plan)
	plan.Warnings = append(plan.Warnings, emailWarnings...)
	if _, listed := localReleaseRowFor(build.Application.Version); !listed {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Magento %s has no tested local row; service defaults come from %s", build.Application.Version, row.Release))
	}
	if !containsBranch(row.PHPBranches, php) {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("PHP %s is outside the tested local row for Magento %s", php, row.Release))
	}
	for _, service := range []LocalServicePlan{database, cache, search, queue, webCache} {
		if service.Image != "" && !strings.Contains(service.Image, "@sha256:") {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("local %s %s %s uses an unpinned upstream image; set local.%s.image to pin a digest", service.Component, service.Family, service.Version, service.Component))
		}
	}
	if hints.Provider != "" {
		plan.Warnings = append(plan.Warnings, CloudOnlyNote)
	}
	if unsupportedCache {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Redis %s is outside the Adobe compatibility snapshot; the current release rows list Valkey instead", cache.Version))
	}
	// Drift direction is cloud-has-less: local runs a container the cloud
	// environment does not have, so local success proves nothing there.
	if strings.EqualFold(strings.TrimSpace(hints.SearchMode), "disabled") {
		plan.Warnings = append(plan.Warnings, "cloud search is disabled for this environment; local OpenSearch does not prove cloud behavior")
	}
	switch strings.ToLower(strings.TrimSpace(hints.QueueMode)) {
	case "db", "database":
		if queue.Family == "database" {
			break
		}
		plan.Warnings = append(plan.Warnings, "cloud queues are database-backed for this environment; the local broker does not prove cloud behavior")
	}
	return plan, nil
}

func sessionPlanFromCache(cache LocalServicePlan) LocalServicePlan {
	session := cache
	session.Component = "session"
	session.Connection.Host = "session"
	return session
}

func chooseCache(requested config.LocalService, rows []localServiceRow) (LocalServicePlan, bool, error) {
	if strings.EqualFold(strings.TrimSpace(requested.Family), "redis") {
		row := cacheRows(localServiceRow{Family: "redis", Version: "7.2", Image: localRedis72Image})[0]
		plan, err := chooseService("cache", requested, []localServiceRow{row})
		return plan, true, err
	}
	plan, err := chooseService("cache", requested, rows)
	return plan, false, err
}

func chooseWebServer(requested config.LocalService, webRuntime, php string) (LocalServicePlan, error) {
	webRuntime = strings.TrimSpace(webRuntime)
	if webRuntime == "" {
		webRuntime = "nginx-fpm"
	}
	var expectedFamily, version, image string
	switch webRuntime {
	case "nginx-fpm":
		expectedFamily, version, image = "nginx", "1.30", "magelift/php-nginx:"+phpBranch(php)+"-local"
	case "frankenphp-classic":
		expectedFamily, version, image = "frankenphp-classic", "1.13.5", "magelift/frankenphp-classic:"+phpBranch(php)+"-local"
	case "php-apache":
		expectedFamily, version, image = "php-apache", "2.4", "magelift/php-apache:"+phpBranch(php)+"-local"
	default:
		return LocalServicePlan{}, fmt.Errorf("local web runtime plugin %q is not registered", webRuntime)
	}
	family := strings.ToLower(strings.TrimSpace(requested.Family))
	if family == "" {
		family = expectedFamily
	}
	if family != expectedFamily {
		return LocalServicePlan{}, fmt.Errorf("local web server family %q contradicts application.webRuntime %q; use family=%q", family, webRuntime, expectedFamily)
	}
	if requested.Version != "" && requested.Version != version && requested.Image == "" {
		return LocalServicePlan{}, fmt.Errorf("set local.webServer.image to use the selected web server version %s", requested.Version)
	}
	if requested.Version != "" {
		version = requested.Version
	}
	if requested.Image != "" {
		if !localImageRef.MatchString(requested.Image) {
			return LocalServicePlan{}, errors.New("local.webServer.image must be an OCI image reference")
		}
		image = requested.Image
	}
	return LocalServicePlan{
		Component:   "web-server",
		Family:      expectedFamily,
		Version:     version,
		Image:       image,
		Ports:       []int{8080},
		Healthcheck: "curl -fsS http://127.0.0.1:8080/health",
		Connection:  LocalConnectionShape{Host: "app", Port: 8080},
	}, nil
}

func normalizeEmailSettings(email config.LocalEmailSettings) (config.LocalEmailSettings, []string, error) {
	mode := strings.ToLower(strings.TrimSpace(email.Mode))
	if mode == "" {
		mode = "disabled"
	}
	if mode == "mailpit" {
		email.Mode = "mailpit"
		if strings.TrimSpace(email.Host) == "" {
			email.Host = "mailpit"
		}
		if email.Port == 0 {
			email.Port = 1025
		}
		if strings.TrimSpace(email.From) == "" {
			email.From = "shop@magelift.test"
		}
		return email, []string{"local Mailpit captures SMTP on loopback; this is not cloud delivery"}, nil
	}
	if email.CredentialEnv != "" && !localCredentialEnvName.MatchString(email.CredentialEnv) {
		return config.LocalEmailSettings{}, nil, fmt.Errorf("local email credentialEnv %q must be an uppercase environment variable name", email.CredentialEnv)
	}
	email.Mode = mode
	warnings := []string{}
	switch mode {
	case "disabled":
		return email, warnings, nil
	case "smtp":
		if strings.TrimSpace(email.Host) == "" || email.Port == 0 {
			return config.LocalEmailSettings{}, nil, errors.New("local email mode smtp requires host and port")
		}
	case "ses":
		if strings.TrimSpace(email.Host) == "" || email.Port == 0 || strings.TrimSpace(email.Username) == "" || email.CredentialEnv == "" {
			return config.LocalEmailSettings{}, nil, errors.New("local email mode ses requires host, port, username, and credentialEnv for SES SMTP credentials")
		}
	default:
		return config.LocalEmailSettings{}, nil, fmt.Errorf("local email mode %q is not verified; use disabled, mailpit, smtp, or ses", mode)
	}
	warnings = append(warnings, "local email uses Magento's SMTP transport; delivery is verified as configuration wiring, not as an external SES delivery claim")
	return email, warnings, nil
}

func sortedLocalExtensions() []string {
	values := make([]string, 0, len(localSupportedExtensions))
	for extension := range localSupportedExtensions {
		values = append(values, extension)
	}
	sort.Strings(values)
	return values
}

func localReleaseRowFor(version string) (localReleaseRow, bool) {
	row := defaultServices()
	line, _, _ := strings.Cut(version, "-")
	for _, recommendation := range config.CurrentCompatibilityCatalog().Lines {
		if recommendation.Line != line {
			continue
		}
		row.Release, row.PHPBranches = recommendation.RecommendedRelease, recommendation.PHPBranches
		// These are convenient local defaults. Individual project service
		// versions override each value independently.
		if line == "2.4.7" {
			row.Database = append(databaseRows(localServiceRow{Family: "mariadb", Version: "10.11", Image: localMariaDB1011Image}), row.Database...)
			row.Search = append(searchRows(localServiceRow{Family: "opensearch", Version: "2", Image: localOpenSearch2Image}), row.Search...)
		}
		if line == "2.4.7" || line == "2.4.8" {
			row.Cache[0].Version, row.Cache[0].Image = "8.1", localValkey81Image
		}
		return row, true
	}
	return row, false
}

var imageVersion = regexp.MustCompile(`^[0-9][a-zA-Z0-9._-]*$`)
var localImageRef = regexp.MustCompile(`^[a-z0-9][A-Za-z0-9./:@_-]*$`)

func chooseService(component string, requested config.LocalService, rows []localServiceRow) (LocalServicePlan, error) {
	family := strings.ToLower(strings.TrimSpace(requested.Family))
	if component == "queue" && (family == "database" || family == "db") {
		return LocalServicePlan{Component: "queue", Family: "database", Connection: LocalConnectionShape{Host: "database", Database: "magento"}}, nil
	}
	if requested.Image != "" && !localImageRef.MatchString(requested.Image) {
		return LocalServicePlan{}, fmt.Errorf("local %s image must be a single OCI image reference", component)
	}
	version := strings.TrimSpace(requested.Version)
	if version != "" && !imageVersion.MatchString(version) {
		return LocalServicePlan{}, fmt.Errorf("local %s has invalid image version %q", component, version)
	}
	for _, row := range rows {
		if family != "" && family != row.Family {
			continue
		}
		if version != "" && version != row.Version {
			tag, _, _ := strings.Cut(row.Image, "@")
			end := strings.LastIndex(tag, ":")
			if end <= 0 {
				continue
			}
			suffix := map[string]string{"rabbitmq": "-management", "artemis": "-alpine"}[row.Family]
			imageTag := version
			if row.Family == "opensearch" {
				imageTag = sdk.OpenSearchContainerTag(version)
			}
			row.Image, row.Version = tag[:end+1]+imageTag+suffix, version
		}
		if requested.Image != "" {
			row.Image = requested.Image
		}
		return localServicePlan(component, row), nil
	}
	return LocalServicePlan{}, fmt.Errorf("local %s family %q has no connection adapter", component, family)
}

func localServicePlan(component string, row localServiceRow) LocalServicePlan {
	return LocalServicePlan{
		Component:   component,
		Family:      row.Family,
		Version:     row.Version,
		Image:       row.Image,
		Ports:       append([]int(nil), row.Ports...),
		Healthcheck: row.Healthcheck,
		Credentials: append([]LocalCredentialBinding(nil), row.Credentials...),
		Connection:  row.Connection,
	}
}

func containsBranch(branches []string, version string) bool {
	branch := phpBranch(version)
	for _, candidate := range branches {
		if candidate == branch {
			return true
		}
	}
	return false
}

func phpBranch(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return strings.TrimSpace(version)
	}
	return parts[0] + "." + parts[1]
}

func serviceVersionMatches(requested, catalogued string) bool {
	if requested == catalogued {
		return true
	}
	return len(catalogued) == 1 && strings.HasPrefix(requested, catalogued+".")
}

func ComposeTemplateFor(plan RuntimePlan) string {
	template := ComposeTemplate
	if plan.Queue.Family == "database" {
		template = strings.Replace(template, "      queue:\n        condition: service_healthy\n", "", 1)
		template = strings.TrimSuffix(template, "  queue:\n")
	}
	template = strings.Replace(template, localMySQL84Image, plan.Database.Image, 1)
	template = strings.Replace(template, localValkey9Image, plan.Cache.Image, 1)
	template = strings.Replace(template, localOpenSearch3Image, plan.Search.Image, 1)
	template = strings.Replace(template, "magelift/php-nginx:8.5-local", plan.AppImage, 1)
	template = strings.Replace(template, "__MAGELIFT_QUEUE_SERVICE__", composeQueueService(plan.Queue), 1)
	template = strings.Replace(template, "__MAGELIFT_SESSION_SERVICE__", composeSessionService(plan), 1)
	template = strings.Replace(template, "__MAGELIFT_MAILPIT_SERVICE__", composeMailpitService(plan.Email), 1)
	template = strings.Replace(template, "__MAGELIFT_MAILPIT_DEPENDS__", composeMailpitDepends(plan.Email), 1)
	template = strings.Replace(template, "__MAGELIFT_SESSION_DEPENDS__", composeSessionDepends(plan), 1)
	template = strings.Replace(template, "__MAGELIFT_SESSION_VOLUME__", composeSessionVolume(plan), 1)
	template = strings.Replace(template, "__MAGELIFT_VARNISH_SERVICE__", composeVarnishService(plan), 1)
	template = strings.Replace(template, `    command: ["php-fpm", "--nodaemonize"]`, composeAppCommand(plan), 1)
	if plan.Cache.Family == "redis" {
		template = strings.Replace(template, `test: ["CMD", "valkey-cli", "ping"]`, `test: ["CMD", "redis-cli", "ping"]`, 1)
	}
	if plan.WebCache.Family == "varnish" {
		template = strings.Replace(template, "MAGELIFT_LOCAL_APP_HTTP_PORT:-8080", "MAGELIFT_LOCAL_APP_HTTP_PORT:-8081", 1)
	}
	emailValues := localEmailRuntimeValues(plan.Email)
	queueAMQPHost, queueAMQPPort := plan.Queue.Connection.Host, plan.Queue.Connection.Port
	if plan.Queue.Family == "artemis" {
		queueAMQPHost, queueAMQPPort = "queue", 5672
	}
	values := map[string]string{
		"MAGELIFT_LOCAL_MAGENTO_VERSION":                                  plan.MagentoVersion,
		"MAGELIFT_LOCAL_PHP":                                              plan.PHP,
		"MAGELIFT_LOCAL_COMPOSER":                                         plan.Composer,
		"MAGELIFT_LOCAL_EXTENSIONS":                                       strings.Join(plan.Extensions, ","),
		"MAGELIFT_LOCAL_PHP_SETTINGS":                                     serializePHPSettings(plan.PHPSettings),
		"MAGELIFT_LOCAL_DATABASE_FAMILY":                                  plan.Database.Family,
		"MAGELIFT_LOCAL_DATABASE_VERSION":                                 plan.Database.Version,
		"MAGELIFT_LOCAL_CACHE_FAMILY":                                     plan.Cache.Family,
		"MAGELIFT_LOCAL_CACHE_VERSION":                                    plan.Cache.Version,
		"MAGELIFT_LOCAL_SEARCH_FAMILY":                                    plan.Search.Family,
		"MAGELIFT_LOCAL_SEARCH_VERSION":                                   plan.Search.Version,
		"MAGELIFT_LOCAL_QUEUE_FAMILY":                                     plan.Queue.Family,
		"MAGELIFT_LOCAL_QUEUE_VERSION":                                    plan.Queue.Version,
		"MAGELIFT_LOCAL_WEB_SERVER_FAMILY":                                plan.WebServer.Family,
		"MAGELIFT_LOCAL_WEB_SERVER_VERSION":                               plan.WebServer.Version,
		"MAGELIFT_LOCAL_WEB_CACHE_FAMILY":                                 plan.WebCache.Family,
		"MAGELIFT_LOCAL_WEB_CACHE_VERSION":                                plan.WebCache.Version,
		"MAGELIFT_LOCAL_EMAIL_MODE":                                       plan.Email.Mode,
		"MAGELIFT_LOCAL_EMAIL_HOST":                                       plan.Email.Host,
		"MAGELIFT_LOCAL_EMAIL_PORT":                                       strconv.Itoa(plan.Email.Port),
		"MAGELIFT_LOCAL_EMAIL_USERNAME":                                   plan.Email.Username,
		"MAGELIFT_LOCAL_EMAIL_FROM":                                       plan.Email.From,
		"MAGELIFT_LOCAL_EMAIL_CREDENTIAL_ENV":                             plan.Email.CredentialEnv,
		"MAGELIFT_LOCAL_EMAIL_DISABLE":                                    emailValues["disable"],
		"MAGELIFT_LOCAL_EMAIL_TRANSPORT":                                  emailValues["transport"],
		"MAGELIFT_LOCAL_EMAIL_AUTH":                                       emailValues["auth"],
		"MAGELIFT_LOCAL_EMAIL_SSL":                                        emailValues["ssl"],
		"MAGELIFT_LOCAL_EMAIL_SET_RETURN_PATH":                            emailValues["set_return_path"],
		"MAGELIFT_LOCAL_EMAIL_RETURN_PATH_EMAIL":                          emailValues["return_path_email"],
		"MAGENTO_DC__OVERRIDE":                                            platform.MergeDeploymentOverride(localDeploymentConfigOverride(plan), platform.MagentoOverlayJSON(plan.Magento)),
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST":                        plan.Database.Connection.Host,
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME":                      plan.Database.Connection.Database,
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME":                    plan.Database.Connection.User,
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD":                    "magento",
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__MODEL":                       "mysql4",
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__ENGINE":                      "innodb",
		"MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__SERVER":    plan.Cache.Connection.Host,
		"MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__PORT":      strconv.Itoa(plan.Cache.Connection.Port),
		"MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__SERVER": plan.Cache.Connection.Host,
		"MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__PORT":   strconv.Itoa(plan.Cache.Connection.Port),
		"MAGENTO_DC_SESSION__SAVE":                                        localSessionSave(plan),
		"MAGENTO_DC_SESSION__REDIS_HOST":                                  localSessionHost(plan),
		"MAGENTO_DC_SESSION__REDIS_PORT":                                  localSessionPort(plan),
		"MAGENTO_DC_CATALOG__SEARCH__ENGINE":                              "opensearch",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME":          plan.Search.Connection.Host,
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT":              strconv.Itoa(plan.Search.Connection.Port),
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH":              "0",
		"CONFIG__DEFAULT__CATALOG__SEARCH__ENGINE":                        "opensearch",
		"CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME":    plan.Search.Connection.Host,
		"CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_PORT":        strconv.Itoa(plan.Search.Connection.Port),
		"CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH":        "0",
		"MAGENTO_DC_QUEUE__DEFAULT_CONNECTION":                            localQueueConnection(plan.Queue),
		"MAGENTO_DC_QUEUE__AMQP__HOST":                                    queueAMQPHost,
		"MAGENTO_DC_QUEUE__AMQP__PORT":                                    strconv.Itoa(queueAMQPPort),
		"MAGENTO_DC_QUEUE__AMQP__SSL":                                     "0",
		"MAGENTO_DC_QUEUE__AMQP__USERNAME":                                valueOrDefault(plan.Queue.Connection.User, "magento"),
		"MAGENTO_DC_QUEUE__AMQP__PASSWORD":                                "magento",
		"MAGENTO_DC_QUEUE__STOMP__HOST":                                   valueOrDefault(plan.Queue.Connection.Host, "queue"),
		"MAGENTO_DC_QUEUE__STOMP__PORT":                                   strconv.Itoa(valueOrDefaultInt(plan.Queue.Connection.Port, 61613)),
		"MAGENTO_DC_QUEUE__STOMP__SSL":                                    "0",
		"MAGENTO_DC_QUEUE__STOMP__USER":                                   valueOrDefault(plan.Queue.Connection.User, "magento"),
		"MAGENTO_DC_QUEUE__STOMP__PASSWORD":                               "magento",
	}
	for _, binding := range platform.MagentoOverlayEnv(plan.Magento) {
		values[binding.Name] = binding.Value
	}
	for key, value := range values {
		template = replaceComposeEnvironment(template, key, value)
	}
	var extra []string
	for _, binding := range platform.MagentoOverlayEnv(plan.Magento) {
		if strings.Contains(template, "      "+binding.Name+": ") {
			continue
		}
		extra = append(extra, "      "+binding.Name+": "+strconv.Quote(binding.Value))
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		command := composeAppCommand(plan)
		template = strings.Replace(template, command, strings.Join(extra, "\n")+"\n"+command, 1)
	}
	return template
}

func composeQueueService(queue LocalServicePlan) string {
	if queue.Family == "database" {
		return ""
	}
	if queue.Family == "artemis" {
		return fmt.Sprintf(`  queue:
    profiles: ["queue"]
    image: ${MAGELIFT_LOCAL_QUEUE_IMAGE:-%s}
    environment:
      ARTEMIS_USER: magento
      ARTEMIS_PASSWORD: magento
      ANONYMOUS_LOGIN: "false"
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_QUEUE_PORT:-61613}:61613"
      - "127.0.0.1:${MAGELIFT_LOCAL_QUEUE_MANAGEMENT_PORT:-8161}:8161"
    volumes:
      - queue:/var/lib/artemis-instance
    healthcheck:
      test: ["CMD-SHELL", "%s"]
      interval: 10s
      timeout: 5s
      retries: 30
`, queue.Image, queue.Healthcheck)
	}
	return fmt.Sprintf(`  queue:
    profiles: ["queue"]
    image: ${MAGELIFT_LOCAL_QUEUE_IMAGE:-%s}
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
`, queue.Image)
}

func composeVarnishService(plan RuntimePlan) string {
	if plan.WebCache.Family != "varnish" {
		return ""
	}
	return fmt.Sprintf(`  varnish:
    profiles: ["web-cache"]
    image: ${MAGELIFT_LOCAL_WEB_CACHE_IMAGE:-%s}
    environment:
      VARNISH_BACKEND_HOST: app
      VARNISH_BACKEND_PORT: "8080"
    depends_on:
      app:
        condition: service_healthy
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_HTTP_PORT:-8080}:80"
    healthcheck:
      test: ["CMD", "varnishadm", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20
`, plan.WebCache.Image)
}

func composeSessionService(plan RuntimePlan) string {
	if plan.Session.Family == "" {
		return ""
	}
	health := "valkey-cli"
	if plan.Session.Family == "redis" {
		health = "redis-cli"
	}
	return fmt.Sprintf(`  session:
    image: ${MAGELIFT_LOCAL_SESSION_IMAGE:-%s}
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_SESSION_PORT:-6380}:6379"
    volumes:
      - session:/data
    healthcheck:
      test: ["CMD", "%s", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

`, plan.Session.Image, health)
}

func composeSessionDepends(plan RuntimePlan) string {
	if plan.Session.Family == "" {
		return ""
	}
	return `      session:
        condition: service_healthy
`
}

func composeSessionVolume(plan RuntimePlan) string {
	if plan.Session.Family == "" {
		return ""
	}
	return "  session:\n"
}

func localSessionSave(plan RuntimePlan) string {
	if plan.Session.Family == "" {
		return "files"
	}
	return "redis"
}

func localSessionHost(plan RuntimePlan) string {
	if plan.Session.Family == "" {
		return ""
	}
	return plan.Session.Connection.Host
}

func localSessionPort(plan RuntimePlan) string {
	if plan.Session.Family == "" {
		return "0"
	}
	return strconv.Itoa(plan.Session.Connection.Port)
}

func composeMailpitService(email config.LocalEmailSettings) string {
	if email.Mode != "mailpit" {
		return ""
	}
	return fmt.Sprintf(`  mailpit:
    profiles: ["email"]
    image: ${MAGELIFT_LOCAL_MAILPIT_IMAGE:-%s}
    ports:
      - "127.0.0.1:${MAGELIFT_LOCAL_MAILPIT_SMTP_PORT:-1025}:1025"
      - "127.0.0.1:${MAGELIFT_LOCAL_MAILPIT_UI_PORT:-8025}:8025"
    healthcheck:
      test: ["CMD", "/mailpit", "readyz"]
      interval: 5s
      timeout: 3s
      retries: 20
`, localMailpitImage)
}

func composeMailpitDepends(email config.LocalEmailSettings) string {
	if email.Mode != "mailpit" {
		return ""
	}
	return `      mailpit:
        condition: service_healthy
`
}

func composeAppCommand(plan RuntimePlan) string {
	switch plan.WebServer.Family {
	case "nginx":
		return `    command: ["sh", "-eu", "-c", "php-fpm --daemonize && exec nginx -g 'daemon off;'"]`
	case "frankenphp-classic":
		return `    command: ["frankenphp", "run"]`
	case "php-apache":
		return `    command: ["sh", "-eu", "-c", "php-fpm --daemonize && exec apache2ctl -D FOREGROUND"]`
	default:
		return `    command: ["false"]`
	}
}

func localEmailRuntimeValues(email config.LocalEmailSettings) map[string]string {
	values := map[string]string{
		"disable":           "1",
		"transport":         "smtp",
		"auth":              "NONE",
		"ssl":               "",
		"set_return_path":   "2",
		"return_path_email": email.From,
	}
	if email.Mode == "disabled" {
		return values
	}
	values["disable"] = "0"
	if email.Username != "" {
		values["auth"] = "LOGIN"
	}
	if email.Mode == "ses" {
		values["auth"] = "LOGIN"
		values["ssl"] = "tls"
	}
	return values
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func valueOrDefaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func localQueueConnection(queue LocalServicePlan) string {
	if queue.Family == "database" {
		return "db"
	}
	if queue.Family == "artemis" {
		return "stomp"
	}
	return "amqp"
}

func localDeploymentConfigOverride(plan RuntimePlan) string {
	queue := map[string]any{
		"default_connection": localQueueConnection(plan.Queue),
	}
	if queue["default_connection"] == "amqp" {
		queue["amqp"] = map[string]any{
			"host":     plan.Queue.Connection.Host,
			"port":     strconv.Itoa(plan.Queue.Connection.Port),
			"ssl":      "0",
			"user":     plan.Queue.Connection.User,
			"password": "magento",
		}
	}
	if queue["default_connection"] == "stomp" {
		queue["stomp"] = map[string]any{
			"host":     plan.Queue.Connection.Host,
			"port":     strconv.Itoa(plan.Queue.Connection.Port),
			"ssl":      "0",
			"user":     plan.Queue.Connection.User,
			"password": "magento",
		}
	}
	email := localEmailDeploymentConfig()
	configuration := map[string]any{
		"db": map[string]any{
			"connection": map[string]any{
				"default": map[string]any{
					"host":           plan.Database.Connection.Host,
					"dbname":         plan.Database.Connection.Database,
					"username":       plan.Database.Connection.User,
					"password":       "magento",
					"model":          "mysql4",
					"engine":         "innodb",
					"initStatements": "SET NAMES utf8;",
					"active":         "1",
				},
			},
		},
		"cache": map[string]any{
			"frontend": map[string]any{
				"default": map[string]any{
					"backend": "Magento\\Framework\\Cache\\Backend\\Redis",
					"backend_options": map[string]any{
						"server":   plan.Cache.Connection.Host,
						"port":     strconv.Itoa(plan.Cache.Connection.Port),
						"database": "0",
					},
				},
				"page_cache": map[string]any{
					"backend": "Magento\\Framework\\Cache\\Backend\\Redis",
					"backend_options": map[string]any{
						"server":   plan.Cache.Connection.Host,
						"port":     strconv.Itoa(plan.Cache.Connection.Port),
						"database": "1",
					},
				},
			},
		},
		"system": map[string]any{
			"default": map[string]any{
				"catalog": map[string]any{
					"search": map[string]any{
						"engine":                     "opensearch",
						"opensearch_server_hostname": plan.Search.Connection.Host,
						"opensearch_server_port":     strconv.Itoa(plan.Search.Connection.Port),
						"opensearch_index_prefix":    "magento2",
						"opensearch_enable_auth":     "0",
						"opensearch_server_timeout":  "15",
					},
				},
				"smile_elasticsuite_core_base_settings": map[string]any{
					"es_client": map[string]any{
						"servers":           plan.Search.Connection.Host + ":" + strconv.Itoa(plan.Search.Connection.Port),
						"enable_https_mode": "0",
						"enable_http_auth":  "0",
					},
				},
				"smtp":        email["smtp"],
				"trans_email": email["trans_email"],
			},
		},
		"queue": queue,
	}
	if plan.Session.Family != "" {
		configuration["session"] = map[string]any{
			"save": "redis",
			"redis": map[string]any{
				"host":     plan.Session.Connection.Host,
				"port":     strconv.Itoa(plan.Session.Connection.Port),
				"database": "0",
			},
		}
	}
	encoded, _ := json.Marshal(configuration)
	return string(encoded)
}

func localEmailDeploymentConfig() map[string]any {
	return map[string]any{
		"smtp": map[string]any{
			"disable":           `#env(MAGELIFT_LOCAL_EMAIL_DISABLE, "1")`,
			"transport":         `#env(MAGELIFT_LOCAL_EMAIL_TRANSPORT, "smtp")`,
			"host":              `#env(MAGELIFT_LOCAL_EMAIL_HOST)`,
			"port":              `#env(MAGELIFT_LOCAL_EMAIL_PORT, "0")`,
			"username":          `#env(MAGELIFT_LOCAL_EMAIL_USERNAME)`,
			"password":          `#env(MAGELIFT_LOCAL_EMAIL_PASSWORD)`,
			"auth":              `#env(MAGELIFT_LOCAL_EMAIL_AUTH, "NONE")`,
			"ssl":               `#env(MAGELIFT_LOCAL_EMAIL_SSL)`,
			"set_return_path":   `#env(MAGELIFT_LOCAL_EMAIL_SET_RETURN_PATH, "2")`,
			"return_path_email": `#env(MAGELIFT_LOCAL_EMAIL_RETURN_PATH_EMAIL)`,
		},
		"trans_email": map[string]any{
			"ident_general": map[string]any{
				"email": `#env(MAGELIFT_LOCAL_EMAIL_FROM)`,
			},
		},
	}
}

// PHPIniFor renders only explicit local overrides. The image owns common
// defaults; this file is the project-specific layer mounted by Compose.
func PHPIniFor(plan RuntimePlan) (string, error) {
	keys := make([]string, 0, len(plan.PHPSettings))
	for key := range plan.PHPSettings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	for _, key := range keys {
		value := plan.PHPSettings[key]
		if !localPHPSettingName.MatchString(key) {
			return "", fmt.Errorf("local PHP setting %q is not a lowercase setting name", key)
		}
		if strings.ContainsAny(value, "\x00\r\n$=;#") {
			return "", fmt.Errorf("local PHP setting %q contains a forbidden character", key)
		}
		output.WriteString(key)
		output.WriteString(" = ")
		output.WriteString(value)
		output.WriteByte('\n')
	}
	return output.String(), nil
}

func serializePHPSettings(settings map[string]string) string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+settings[key])
	}
	return strings.Join(values, ";")
}

func replaceComposeEnvironment(template, key, value string) string {
	prefix := "      " + key + ": "
	start := strings.Index(template, prefix)
	if start < 0 {
		return template
	}
	end := strings.IndexByte(template[start:], '\n')
	if end < 0 {
		end = len(template)
	} else {
		end += start
	}
	return template[:start] + prefix + strconv.Quote(value) + template[end:]
}
