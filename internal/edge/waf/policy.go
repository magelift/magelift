// Package waf is the Magento-safe WAF contract every edge provider must use.
//
// Unmodified vendor defaults are not Magento-safe. Magento 2 admin WYSIWYG,
// catalog import, REST/GraphQL, and media uploads trip OWASP/AWS/GCP body
// signatures. Public guidance:
//
//   - Adobe Fastly WAF is a Magento-tuned OWASP/Trustwave policy on
//     origin-bound cache misses, including admin; false positives are expected
//     and treated as P1. Do not wholesale-bypass /graphql by default.
//     https://experienceleague.adobe.com/en/docs/commerce-on-cloud/user-guide/cdn/fastly-waf-service
//     https://experienceleague.adobe.com/en/docs/commerce-knowledge-base/kb/how-to/how-to-bypass-waf-for-graphql-requests
//   - AWS Magento setups start with CommonRuleSet, SQLi, and KnownBadInputs,
//     then count-override the upload/XSS/SQLi BODY rules instead of disabling
//     the group. CloudFront body inspection defaults to 16 KB and should be
//     raised to 64 KB. Magento admin URLs are customized, so do not hardcode
//     /admin in rate-limit paths.
//     https://www.mgt-commerce.com/tutorial/aws-waf-magento-configuration/
//     https://repost.aws/knowledge-center/waf-upload-blocked-files
//     https://docs.aws.amazon.com/waf/latest/developerguide/web-acl-setting-body-inspection-limit.html
//   - Cloud Armor defaults to sensitivity 4 (OWASP PL4). Magento needs
//     sensitivity 1 plus STANDARD_WITH_GRAPHQL JSON parsing and 64 KB bodies.
//     https://docs.cloud.google.com/armor/docs/rule-tuning
//     https://docs.cloud.google.com/armor/docs/content-parsing
//
// Tenant IP allowlists are not portable and are not part of this policy.
package waf

import "strings"

const (
	// PolicyRef is the portable WAF policy identity for Magento-safe managed
	// rules. Provider adapters translate this; they must not attach an
	// unmodified vendor default ruleset.
	PolicyRef = "waf/magento-safe"

	// RateLimitPerIP is the site-wide Magento rate limit (requests / 5 min).
	// Magento admin paths are customized per shop, so this is not an /admin
	// login throttle.
	RateLimitPerIP = 2000
	// RateLimitWindowSeconds is AWS WAF's default evaluation window.
	RateLimitWindowSeconds = 300
	// BodyInspectionKB is CloudFront/Cloud Armor's maximum inspectable body.
	// Magento admin uploads and product saves exceed the vendor 8–16 KB default.
	BodyInspectionKB = 64
	// ScalewayMaxParanoiaLevel is vanilla OWASP CRS paranoia 1. Adobe's
	// Fastly Magento WAF can run Magento-tuned rules up to PL3; generic CRS
	// PL2+ is not that ruleset and false-positives Magento admin/GraphQL.
	ScalewayMaxParanoiaLevel = 1
)

// AWSManagedGroup is one AWS managed rule group with Magento count-overrides.
// Remaining rules in the group keep their vendor block action.
type AWSManagedGroup struct {
	Name           string
	Priority       int
	CountOverrides []string
}

// ArmorWAFSet is one Cloud Armor preconfigured WAF ruleset mapped from the
// Magento AWS exclusions. Body/URI/query exclusions are the Armor equivalent
// of AWS count-overrides on BODY/URI/QUERYARGUMENT rules.
type ArmorWAFSet struct {
	Name           string
	Priority       int
	RuleSet        string
	Preview        bool
	ExcludeBody    bool
	ExcludeURI     bool
	ExcludeQuery   bool
	ExcludeCookies bool
}

// FastlyPathExclusion is a Magento path that must not be inspected by a
// generic OWASP/NGWAF ruleset. Admin, GraphQL, and REST stay in scope.
type FastlyPathExclusion struct {
	PathPrefix string
	Reason     string
}

// Policy is the Magento-safe WAF profile.
type Policy struct {
	RateLimitPerIP         int
	RateLimitWindowSeconds int
	BodyInspectionKB       int
	AdminPathPrefix        string
	AWSManagedGroups       []AWSManagedGroup
	ArmorWAFSets           []ArmorWAFSet
	FastlyPathExclusions   []FastlyPathExclusion
	ScalewayMaxParanoia    int
}

// MagentoSafe is the production Magento WAF profile for the default admin
// frontName. Prefer MagentoSafeFor when YAML configures application.magento.frontName.
func MagentoSafe() Policy {
	return MagentoSafeFor("admin")
}

// AdminPathPrefix returns the Magento admin URI prefix for a configured frontName.
func AdminPathPrefix(frontName string) string {
	name := strings.Trim(strings.TrimSpace(frontName), "/")
	if name == "" {
		name = "admin"
	}
	return "/" + name
}

// MagentoSafeFor is the production Magento WAF profile. Group override action
// is none (block) except the listed Magento false-positive rules, which count.
// Admin stays in inspection scope; the configured frontName is recorded so
// adapters never hardcode /admin in rate-limit or URI matches.
func MagentoSafeFor(frontName string) Policy {
	return Policy{
		RateLimitPerIP:         RateLimitPerIP,
		RateLimitWindowSeconds: RateLimitWindowSeconds,
		BodyInspectionKB:       BodyInspectionKB,
		AdminPathPrefix:        AdminPathPrefix(frontName),
		ScalewayMaxParanoia:    ScalewayMaxParanoiaLevel,
		AWSManagedGroups: []AWSManagedGroup{
			{Name: "AWSManagedRulesAmazonIpReputationList", Priority: 2},
			{
				Name:     "AWSManagedRulesCommonRuleSet",
				Priority: 3,
				CountOverrides: []string{
					// AWS SizeRestrictions_BODY blocks bodies over 8 KB; Magento
					// catalog import, CMS, and GraphQL exceed that.
					"SizeRestrictions_BODY",
					// Magento WYSIWYG/TinyMCE posts HTML for CMS and products.
					"CrossSiteScripting_BODY",
					// CMS and config fields contain storefront/CDN URLs.
					"GenericRFI_BODY",
					// Magento media/CSV uploads match LFI/SSRF byte patterns.
					"GenericLFI_BODY",
					"EC2MetaDataSSRF_BODY",
					// Magento cron and some API clients omit User-Agent.
					"NoUserAgent_HEADER",
				},
			},
			{
				Name:     "AWSManagedRulesKnownBadInputsRuleSet",
				Priority: 4,
				CountOverrides: []string{
					// Admin POST bodies match Log4j/JNDI signatures in free text.
					"Log4JRCE_BODY",
				},
			},
			{
				Name:     "AWSManagedRulesSQLiRuleSet",
				Priority: 5,
				CountOverrides: []string{
					// Product descriptions and CMS contain SQL-like keywords.
					"SQLi_BODY",
					// Admin grid filters put SQL-like patterns in query args.
					"SQLiExtendedPatterns_QUERYARGUMENTS",
				},
			},
			{
				Name:     "AWSManagedRulesPHPRuleSet",
				Priority: 6,
				CountOverrides: []string{
					// Admin POSTs include serialized PHP and function names.
					"PHPHighRiskMethodsVariables_BODY",
				},
			},
			{
				Name:     "AWSManagedRulesLinuxRuleSet",
				Priority: 7,
				CountOverrides: []string{
					// Magento admin URIs look like file paths.
					"LFI_URIPATH",
					// Cron/config query strings look like /var/log paths.
					"LFI_QUERYSTRING",
				},
			},
		},
		ArmorWAFSets: []ArmorWAFSet{
			{Name: "magento-sqli", Priority: 1000, RuleSet: "sqli-v33-stable", ExcludeBody: true, ExcludeQuery: true},
			{Name: "magento-xss", Priority: 1010, RuleSet: "xss-v33-stable", ExcludeBody: true},
			{Name: "magento-lfi", Priority: 1020, RuleSet: "lfi-v33-stable", ExcludeBody: true, ExcludeURI: true, ExcludeQuery: true},
			{Name: "magento-rfi", Priority: 1030, RuleSet: "rfi-v33-stable", ExcludeBody: true},
			{Name: "magento-php", Priority: 1040, RuleSet: "php-v33-stable", ExcludeBody: true},
			{Name: "magento-java", Priority: 1050, RuleSet: "java-v33-stable", ExcludeBody: true},
			{Name: "magento-session", Priority: 1060, RuleSet: "sessionfixation-v33-stable", Preview: true, ExcludeCookies: true},
			{Name: "scannerdetection", Priority: 2000, RuleSet: "scannerdetection-v33-stable"},
			{Name: "protocolattack", Priority: 2010, RuleSet: "protocolattack-v33-stable"},
		},
		FastlyPathExclusions: []FastlyPathExclusion{
			{PathPrefix: "/static/", Reason: "Magento static assets; generic OWASP inspects hashed URLs as LFI"},
			{PathPrefix: "/media/", Reason: "Magento media; uploaded files trip LFI/XSS/size rules"},
			{PathPrefix: "/pub/static/", Reason: "Magento pub/static when rewrite is not in front"},
			{PathPrefix: "/pub/media/", Reason: "Magento pub/media when rewrite is not in front"},
			{PathPrefix: "/health_check", Reason: "Magento/Fastly health probe"},
		},
	}
}
