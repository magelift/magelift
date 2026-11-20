package paasimport

// allowlistedEnv contains source variables that the importers translate.
// The import report lists keys outside this set.
var allowlistedEnv = map[string]struct{}{
	"CRYPT_KEY":       {},
	"SCD_STRATEGY":    {},
	"SCD_THREADS":     {},
	"UPDATE_URLS":     {},
	"QUALITY_PATCHES": {},
	"CONFIG__DEFAULT__ADMIN__URL__FRONT_NAME":            {},
	"CONFIG__DEFAULT__WEB__COOKIE__COOKIE_DOMAIN":        {},
	"CONFIG__DEFAULT__WEB__CORS__ORIGINS":                {},
	"CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ORIGINS":        {},
	"CONFIG__DEFAULT__CRON__CONSUMERS_RUNNER__MODE":      {},
	"CONFIG__DEFAULT__CRON__CONSUMERS_RUNNER__CONSUMERS": {},
}

// IsAllowlistedEnv reports whether the importers recognize key.
func IsAllowlistedEnv(key string) bool {
	_, ok := allowlistedEnv[key]
	return ok
}
