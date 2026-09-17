package platform

import (
	"strconv"
	"strings"
)

// Magento / MageLift environment variable names. Adapters inject values from
// their capability endpoints; they must not invent alternate Magento env names.
const (
	EnvApplicationMode = "MAGELIFT_APPLICATION_MODE"
	EnvWebRuntime      = "MAGELIFT_WEB_RUNTIME"

	EnvDatabaseWriter  = "MAGELIFT_DATABASE_WRITER"
	EnvCacheEndpoint   = "MAGELIFT_CACHE_ENDPOINT"
	EnvSessionEndpoint = "MAGELIFT_SESSION_ENDPOINT"

	EnvMagentoDBHost   = "MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST"
	EnvMagentoDBPort   = "MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT"
	EnvMagentoDBName   = "MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME"
	EnvMagentoDBModel  = "MAGENTO_DC_DB__CONNECTION__DEFAULT__MODEL"
	EnvMagentoDBEngine = "MAGENTO_DC_DB__CONNECTION__DEFAULT__ENGINE"
	EnvMagentoDBInit   = "MAGENTO_DC_DB__CONNECTION__DEFAULT__INITSTATEMENTS"
	EnvMagentoDBActive = "MAGENTO_DC_DB__CONNECTION__DEFAULT__ACTIVE"
	EnvMagentoDBUser   = "MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME"
	EnvMagentoDBPass   = "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD"

	EnvMagentoInstallDate = "MAGENTO_DC_INSTALL__DATE"

	EnvMagentoCacheBackend = "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND"
	EnvMagentoCacheServer  = "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__SERVER"
	EnvMagentoCachePort    = "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__PORT"
	EnvMagentoCacheDB      = "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__DATABASE"

	EnvMagentoPageCacheBackend  = "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND"
	EnvMagentoPageCacheServer   = "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__SERVER"
	EnvMagentoPageCachePort     = "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__PORT"
	EnvMagentoPageCacheDB       = "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__DATABASE"
	EnvMagentoPageCacheCompress = "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__COMPRESS_DATA"

	EnvMagentoSessionSave = "MAGENTO_DC_SESSION__SAVE"
	EnvMagentoSessionHost = "MAGENTO_DC_SESSION__REDIS_HOST"
	EnvMagentoSessionPort = "MAGENTO_DC_SESSION__REDIS_PORT"
	EnvMagentoSessionDB   = "MAGENTO_DC_SESSION__REDIS_DB"

	EnvMagentoCryptKey = "MAGENTO_DC_CRYPT__KEY"

	EnvSearchEndpoint = "MAGELIFT_SEARCH_ENDPOINT"
	EnvMediaBucket    = "MAGELIFT_MEDIA_BUCKET"
	EnvMediaURL       = "MAGELIFT_MEDIA_URL"
	// EnvMediaS3* feeds the PHP lifecycle remote-storage writer. The
	// secret travels only via SecretKeyRef (MAGELIFT_MEDIA_S3_SECRET),
	// never as a plain binding.
	EnvMediaS3Key      = "MAGELIFT_MEDIA_S3_KEY"
	EnvMediaS3Endpoint = "MAGELIFT_MEDIA_S3_ENDPOINT"
	EnvMediaS3Region   = "MAGELIFT_MEDIA_S3_REGION"
	EnvMediaS3Prefix   = "MAGELIFT_MEDIA_S3_PREFIX"
	EnvQueueMode       = "MAGELIFT_QUEUE_MODE"

	EnvMagentoSearchEngine      = "MAGENTO_DC_CATALOG__SEARCH__ENGINE"
	EnvMagentoSearchHost        = "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME"
	EnvMagentoSearchPort        = "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT"
	EnvMagentoSearchIndexPrefix = "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_INDEX_PREFIX"
	EnvMagentoSearchEnableAuth  = "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH"
	EnvMagentoSearchTimeout     = "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_TIMEOUT"
	// Magento never resolves #env() placeholders under env.php's system
	// section, so the MAGENTO_DC search bindings above cannot reach
	// catalog/search readers (live reindex saw the raw literals). These
	// CONFIG__ bindings use stock Magento's environment configuration
	// contract, which takes precedence over env.php system values.
	EnvMagentoSearchConfigEngine      = "CONFIG__DEFAULT__CATALOG__SEARCH__ENGINE"
	EnvMagentoSearchConfigHost        = "CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME"
	EnvMagentoSearchConfigPort        = "CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_PORT"
	EnvMagentoSearchConfigIndexPrefix = "CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_INDEX_PREFIX"
	EnvMagentoSearchConfigEnableAuth  = "CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH"
	EnvMagentoSearchConfigTimeout     = "CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_TIMEOUT"
	EnvMagentoDCOverride              = "MAGENTO_DC__OVERRIDE"
	EnvMagentoElasticsuiteServers     = "MAGENTO_DC_ELASTICSUITE__ES_CLIENT__SERVERS"
	EnvMagentoElasticsuiteHTTPS       = "MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTPS_MODE"
	EnvMagentoElasticsuiteAuth        = "MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTP_AUTH"

	EnvMagentoQueueHost     = "MAGENTO_DC_QUEUE__AMQP__HOST"
	EnvMagentoQueuePort     = "MAGENTO_DC_QUEUE__AMQP__PORT"
	EnvMagentoQueueSSL      = "MAGENTO_DC_QUEUE__AMQP__SSL"
	EnvMagentoQueueUsername = "MAGENTO_DC_QUEUE__AMQP__USERNAME"
	EnvMagentoQueuePassword = "MAGENTO_DC_QUEUE__AMQP__PASSWORD"
	EnvMagentoQueueDefault  = "MAGENTO_DC_QUEUE__DEFAULT_CONNECTION"

	DefaultMySQLPort          = "3306"
	DefaultValkeyPort         = "6379"
	DefaultOpenSearchPort     = "9200"
	DefaultAMQPPort           = "5672"
	DefaultMagentoInstallDate = "Thu, 01 Jan 1970 00:00:00 +0000"
	RedisCacheBackend         = "Magento\\Framework\\Cache\\Backend\\Redis"
	ValkeyCacheBackend        = "valkey"
)

// EnvBinding is a literal container environment entry.
type EnvBinding struct {
	Name  string
	Value string
}

// CapabilityEndpoints are resolved hostnames/endpoints from cloud adapters.
type CapabilityEndpoints struct {
	ApplicationMode    string
	WebRuntime         string
	ApplicationVersion string
	DatabaseWriter     string
	DatabaseName       string
	CacheEndpoint      string
	SessionEndpoint    string
	SearchEndpoint     string
	QueueMode          string // database | rabbitmq
	QueueHost          string
	QueueUsername      string
	MediaBucket        string
	MediaURL           string
	MediaS3Key         string
	MediaS3Endpoint    string
	MediaS3Region      string
	MediaS3Prefix      string
	Magento            MagentoOverlays
}

// CoreEnvBindings returns the Magento env contract for database + Valkey cache
// and session. Optional search/queue/media bindings are appended when set.
func CoreEnvBindings(endpoints CapabilityEndpoints) []EnvBinding {
	session := endpoints.SessionEndpoint
	if session == "" {
		session = endpoints.CacheEndpoint
	}
	cacheBackend := RedisCacheBackend
	sessionBackend := "redis"
	if MagentoUsesValkey(endpoints.ApplicationVersion) {
		cacheBackend = ValkeyCacheBackend
		sessionBackend = "valkey"
	}
	bindings := []EnvBinding{
		{Name: EnvApplicationMode, Value: endpoints.ApplicationMode},
		{Name: EnvWebRuntime, Value: endpoints.WebRuntime},
		{Name: EnvDatabaseWriter, Value: endpoints.DatabaseWriter},
		{Name: EnvCacheEndpoint, Value: endpoints.CacheEndpoint},
		{Name: EnvSessionEndpoint, Value: session},
		{Name: EnvMagentoDBHost, Value: endpoints.DatabaseWriter},
		{Name: EnvMagentoDBPort, Value: DefaultMySQLPort},
		{Name: EnvMagentoDBName, Value: endpoints.DatabaseName},
		{Name: EnvMagentoDBModel, Value: "mysql4"},
		{Name: EnvMagentoDBEngine, Value: "innodb"},
		{Name: EnvMagentoDBInit, Value: "SET NAMES utf8;"},
		{Name: EnvMagentoDBActive, Value: "1"},
		// Magento only loads the core command list for an installed deployment.
		// Existing database-backed environments may not carry install/date in the
		// image's env.php, so provide a stable marker through its documented
		// deployment-config environment contract.
		{Name: EnvMagentoInstallDate, Value: DefaultMagentoInstallDate},
		{Name: EnvMagentoCacheBackend, Value: cacheBackend},
		{Name: EnvMagentoCacheServer, Value: endpoints.CacheEndpoint},
		{Name: EnvMagentoCachePort, Value: DefaultValkeyPort},
		{Name: EnvMagentoCacheDB, Value: "0"},
		{Name: EnvMagentoPageCacheBackend, Value: cacheBackend},
		{Name: EnvMagentoPageCacheServer, Value: endpoints.CacheEndpoint},
		{Name: EnvMagentoPageCachePort, Value: DefaultValkeyPort},
		{Name: EnvMagentoPageCacheDB, Value: "1"},
		{Name: EnvMagentoPageCacheCompress, Value: "0"},
		{Name: EnvMagentoSessionSave, Value: sessionBackend},
		{Name: EnvMagentoSessionHost, Value: session},
		{Name: EnvMagentoSessionPort, Value: DefaultValkeyPort},
		{Name: EnvMagentoSessionDB, Value: "2"},
	}
	if endpoints.SearchEndpoint != "" {
		host, port, httpsMode, servers := magentoSearchTarget(endpoints.SearchEndpoint)
		// Native Magento derives the OpenSearch scheme from the hostname's own
		// scheme (SearchClient::buildOSConfig defaults to http). AWS managed
		// domains are HTTPS-only, so an unprefixed hostname makes Magento dial
		// http://host:443 and the domain answers 400 (proven live on mlaw1:
		// setup:upgrade "Unknown 400 error from OpenSearch"). ElasticSuite
		// takes bare host:port plus a separate HTTPS flag, so only the native
		// hostname bindings carry the scheme.
		nativeHost := host
		if httpsMode == "1" && !strings.HasPrefix(nativeHost, "https://") {
			nativeHost = "https://" + nativeHost
		}
		bindings = append(bindings,
			EnvBinding{Name: EnvSearchEndpoint, Value: endpoints.SearchEndpoint},
			EnvBinding{Name: EnvMagentoSearchEngine, Value: "opensearch"},
			EnvBinding{Name: EnvMagentoSearchHost, Value: nativeHost},
			EnvBinding{Name: EnvMagentoSearchPort, Value: port},
			EnvBinding{Name: EnvMagentoSearchIndexPrefix, Value: "magento2"},
			EnvBinding{Name: EnvMagentoSearchEnableAuth, Value: "0"},
			EnvBinding{Name: EnvMagentoSearchTimeout, Value: "15"},
			EnvBinding{Name: EnvMagentoSearchConfigEngine, Value: "opensearch"},
			EnvBinding{Name: EnvMagentoSearchConfigHost, Value: nativeHost},
			EnvBinding{Name: EnvMagentoSearchConfigPort, Value: port},
			EnvBinding{Name: EnvMagentoSearchConfigIndexPrefix, Value: "magento2"},
			EnvBinding{Name: EnvMagentoSearchConfigEnableAuth, Value: "0"},
			EnvBinding{Name: EnvMagentoSearchConfigTimeout, Value: "15"},
			EnvBinding{Name: EnvMagentoElasticsuiteServers, Value: servers},
			EnvBinding{Name: EnvMagentoElasticsuiteHTTPS, Value: httpsMode},
			EnvBinding{Name: EnvMagentoElasticsuiteAuth, Value: "0"},
		)
	}
	queueMode := endpoints.QueueMode
	if queueMode == "" {
		queueMode = "database"
	}
	queueConnection := "db"
	if queueMode == "rabbitmq" {
		queueConnection = "amqp"
	}
	bindings = append(bindings,
		EnvBinding{Name: EnvQueueMode, Value: queueMode},
		EnvBinding{Name: EnvMagentoQueueDefault, Value: queueConnection},
	)
	if queueMode == "rabbitmq" && endpoints.QueueHost != "" {
		user := endpoints.QueueUsername
		if user == "" {
			user = "magento"
		}
		bindings = append(bindings,
			EnvBinding{Name: EnvMagentoQueueHost, Value: endpoints.QueueHost},
			EnvBinding{Name: EnvMagentoQueuePort, Value: DefaultAMQPPort},
			EnvBinding{Name: EnvMagentoQueueSSL, Value: "0"},
			EnvBinding{Name: EnvMagentoQueueUsername, Value: user},
		)
	}
	if endpoints.MediaBucket != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMediaBucket, Value: endpoints.MediaBucket})
	}
	if endpoints.MediaURL != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMediaURL, Value: endpoints.MediaURL})
	}
	if endpoints.MediaS3Key != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMediaS3Key, Value: endpoints.MediaS3Key})
	}
	if endpoints.MediaS3Endpoint != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMediaS3Endpoint, Value: endpoints.MediaS3Endpoint})
	}
	if endpoints.MediaS3Region != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMediaS3Region, Value: endpoints.MediaS3Region})
	}
	if endpoints.MediaS3Prefix != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMediaS3Prefix, Value: endpoints.MediaS3Prefix})
	}
	override := ""
	if endpoints.SearchEndpoint != "" {
		_, _, httpsMode, servers := magentoSearchTarget(endpoints.SearchEndpoint)
		override = magentoElasticsuiteOverride(servers, httpsMode)
	}
	if magentoJSON := MagentoOverlayJSON(endpoints.Magento); magentoJSON != "" {
		override = MergeDeploymentOverride(override, magentoJSON)
	}
	if override != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoDCOverride, Value: override})
	}
	bindings = append(bindings, MagentoOverlayEnv(endpoints.Magento)...)
	return bindings
}

// magentoSearchTarget maps a search endpoint onto Magento/ElasticSuite env
// the same way a shop generates env.php: hostname, port, HTTPS. AWS managed
// domains use 443 inside the VPC with HTTP auth off. Local service names stay
// on 9200 without TLS.
func magentoSearchTarget(endpoint string) (host, port, httpsMode, servers string) {
	host = strings.TrimSpace(endpoint)
	port = DefaultOpenSearchPort
	httpsMode = "0"
	switch {
	case strings.HasPrefix(host, "https://"):
		host = strings.TrimPrefix(host, "https://")
		port = "443"
		httpsMode = "1"
	case strings.HasPrefix(host, "http://"):
		host = strings.TrimPrefix(host, "http://")
		port = "80"
	}
	if cut := strings.IndexAny(host, "/?"); cut >= 0 {
		host = host[:cut]
	}
	if name, explicitPort, ok := strings.Cut(host, ":"); ok {
		host, port = name, explicitPort
	}
	if httpsMode == "0" && (strings.Contains(host, ".es.amazonaws.com") || strings.Contains(host, ".aoss.amazonaws.com")) {
		httpsMode = "1"
		if port == DefaultOpenSearchPort {
			port = "443"
		}
	}
	if port == "443" {
		httpsMode = "1"
	}
	return host, port, httpsMode, host + ":" + port
}

func magentoElasticsuiteOverride(servers, httpsMode string) string {
	if httpsMode != "1" {
		httpsMode = "0"
	}
	return `{"system":{"default":{"smile_elasticsuite_core_base_settings":{"es_client":{"servers":` + strconv.Quote(servers) + `,"enable_https_mode":` + httpsMode + `,"enable_http_auth":0}}}}}`
}

// MagentoUsesValkey reports whether Adobe's current release guidance requires
// Valkey instead of Redis for the cache and session configuration. The
// thresholds are the latest patch releases in each supported 2.4.x line;
// earlier patches retain the legacy Redis contract.
func MagentoUsesValkey(version string) bool {
	base, suffix, _ := strings.Cut(strings.TrimSpace(version), "-")
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return false
	}
	if major > 2 || (major == 2 && minor > 4) || (major == 2 && minor == 4 && patch >= 9) {
		return true
	}
	if major != 2 || minor != 4 {
		return false
	}

	securityPatch := 0
	if suffix != "" {
		if !strings.HasPrefix(suffix, "p") {
			return false
		}
		securityPatch, err = strconv.Atoi(strings.TrimPrefix(suffix, "p"))
		if err != nil {
			return false
		}
	}
	switch patch {
	case 8:
		return securityPatch >= 5
	case 7:
		return securityPatch >= 10
	case 6:
		return securityPatch >= 15
	case 5:
		return securityPatch >= 17
	default:
		return false
	}
}
