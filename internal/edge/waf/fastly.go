package waf

// FastlyNGWAF is the Magento-safe Next-Gen WAF translation. Adobe's Magento
// Fastly WAF inspects origin-bound cache misses including admin, GraphQL, and
// REST. Generic OWASP/NGWAF rulesets false-positive Magento hashed static
// URLs and uploaded media, so those prefixes are skipped. This document is
// not an NGWAF API client; adapters must refuse opaque WAFPolicyRef values
// that are not PolicyRef.
type FastlyNGWAF struct {
	PolicyRef              string
	PathExclusions         []FastlyPathExclusion
	RateLimitPerIP         int
	RateLimitWindowSeconds int
}

// Fastly returns the Magento-safe NGWAF translation.
func Fastly() FastlyNGWAF {
	policy := MagentoSafe()
	return FastlyNGWAF{
		PolicyRef:              PolicyRef,
		PathExclusions:         append([]FastlyPathExclusion(nil), policy.FastlyPathExclusions...),
		RateLimitPerIP:         policy.RateLimitPerIP,
		RateLimitWindowSeconds: policy.RateLimitWindowSeconds,
	}
}
