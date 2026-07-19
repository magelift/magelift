package platform

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

	DefaultMySQLPort  = "3306"
	DefaultValkeyPort = "6379"
	RedisCacheBackend = "Magento\\Framework\\Cache\\Backend\\Redis"
)

// EnvBinding is a literal container environment entry.
type EnvBinding struct {
	Name  string
	Value string
}

// CapabilityEndpoints are resolved hostnames/endpoints from cloud adapters.
type CapabilityEndpoints struct {
	ApplicationMode string
	WebRuntime      string
	DatabaseWriter  string
	DatabaseName    string
	CacheEndpoint   string
	SessionEndpoint string
}

// CoreEnvBindings returns the Magento env contract for database + Valkey cache
// and session. Search, queue, and media bindings remain adapter-local until
// those capabilities are ported.
func CoreEnvBindings(endpoints CapabilityEndpoints) []EnvBinding {
	session := endpoints.SessionEndpoint
	if session == "" {
		session = endpoints.CacheEndpoint
	}
	return []EnvBinding{
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
		{Name: EnvMagentoCacheBackend, Value: RedisCacheBackend},
		{Name: EnvMagentoCacheServer, Value: endpoints.CacheEndpoint},
		{Name: EnvMagentoCachePort, Value: DefaultValkeyPort},
		{Name: EnvMagentoCacheDB, Value: "0"},
		{Name: EnvMagentoPageCacheBackend, Value: RedisCacheBackend},
		{Name: EnvMagentoPageCacheServer, Value: endpoints.CacheEndpoint},
		{Name: EnvMagentoPageCachePort, Value: DefaultValkeyPort},
		{Name: EnvMagentoPageCacheDB, Value: "1"},
		{Name: EnvMagentoPageCacheCompress, Value: "0"},
		{Name: EnvMagentoSessionSave, Value: "redis"},
		{Name: EnvMagentoSessionHost, Value: session},
		{Name: EnvMagentoSessionPort, Value: DefaultValkeyPort},
		{Name: EnvMagentoSessionDB, Value: "2"},
	}
}
