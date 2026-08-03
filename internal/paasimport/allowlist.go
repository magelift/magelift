package paasimport

// D-07 frozen allowlist of store-breaker env vars.
// Everything else is an intentional gap / unmapped residual (ECE-04 docs in 04-06).
var allowlistedEnv = map[string]struct{}{
	"CRYPT_KEY":    {},
	"SCD_STRATEGY": {},
	"SCD_THREADS":  {},
	"UPDATE_URLS":  {},
}

// IsAllowlistedEnv reports whether key is on the D-07 v1 allowlist.
func IsAllowlistedEnv(key string) bool {
	_, ok := allowlistedEnv[key]
	return ok
}
