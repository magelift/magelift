package waf

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	armorJSONParsing         = "STANDARD_WITH_GRAPHQL"
	armorBodyInspectionSize  = "64KB"
	armorStaticMediaPriority = 100
	armorRateLimitPriority   = 200
	armorDefaultPriority     = 2147483647
)

// ArmorPolicy is the Magento-safe Cloud Armor profile: 64 KB GraphQL-aware
// body inspection, static/media bypass, IP rate limit, preconfigured WAF with
// Magento body/URI/query exclusions, and a default allow.
type ArmorPolicy struct {
	JSONParsing            string
	RequestBodyInspection  string
	StaticMediaCEL         string
	RateLimitPerIP         int
	RateLimitWindowSeconds int
	WAFSets                []ArmorWAFSet
}

// Armor returns the Magento-safe Cloud Armor translation.
func Armor() ArmorPolicy {
	policy := MagentoSafe()
	return ArmorPolicy{
		JSONParsing:            armorJSONParsing,
		RequestBodyInspection:  armorBodyInspectionSize,
		StaticMediaCEL:         "request.path.startsWith('/static/') || request.path.startsWith('/media/') || request.path.startsWith('/pub/static/') || request.path.startsWith('/pub/media/')",
		RateLimitPerIP:         policy.RateLimitPerIP,
		RateLimitWindowSeconds: policy.RateLimitWindowSeconds,
		WAFSets:                policy.ArmorWAFSets,
	}
}

// ArmorWAFExpression is the Cloud Armor CEL for a preconfigured WAF set at
// sensitivity 1 (OWASP paranoia 1).
func ArmorWAFExpression(ruleSet string) string {
	return fmt.Sprintf("evaluatePreconfiguredWaf('%s', {'sensitivity': 1})", strings.TrimSpace(ruleSet))
}

func (p ArmorPolicy) StaticMediaPriority() int { return armorStaticMediaPriority }
func (p ArmorPolicy) RateLimitPriority() int   { return armorRateLimitPriority }
func (p ArmorPolicy) DefaultPriority() int     { return armorDefaultPriority }

// ArmorPolicyDocument is the gcloud --file-name JSON for a Magento-safe
// Cloud Armor policy: GraphQL JSON parsing, 64 KB bodies, static/media allow,
// IP throttle, sensitivity-1 preconfigured WAF with Magento exclusions, and
// a default allow. requestBodiesToExclude is the Compute beta REST field;
// GA v1 clients omit it, but Magento POST bodies need it.
type ArmorPolicyDocument struct {
	Description           string               `json:"description,omitempty"`
	Type                  string               `json:"type"`
	AdvancedOptionsConfig armorAdvancedOptions `json:"advancedOptionsConfig"`
	Rules                 []armorRule          `json:"rules"`
}

type armorAdvancedOptions struct {
	JSONParsing               string `json:"jsonParsing"`
	LogLevel                  string `json:"logLevel"`
	RequestBodyInspectionSize string `json:"requestBodyInspectionSize"`
}

type armorRule struct {
	Action                 string          `json:"action"`
	Description            string          `json:"description,omitempty"`
	Priority               int             `json:"priority"`
	Preview                bool            `json:"preview,omitempty"`
	Match                  armorMatch      `json:"match"`
	RateLimitOptions       *armorRateLimit `json:"rateLimitOptions,omitempty"`
	PreconfiguredWafConfig *armorWAFConfig `json:"preconfiguredWafConfig,omitempty"`
}

type armorMatch struct {
	Expr          *armorExpr        `json:"expr,omitempty"`
	VersionedExpr string            `json:"versionedExpr,omitempty"`
	Config        *armorMatchConfig `json:"config,omitempty"`
}

type armorExpr struct {
	Expression string `json:"expression"`
}

type armorMatchConfig struct {
	SrcIPRanges []string `json:"srcIpRanges"`
}

type armorRateLimit struct {
	ConformAction      string             `json:"conformAction"`
	ExceedAction       string             `json:"exceedAction"`
	EnforceOnKey       string             `json:"enforceOnKey"`
	RateLimitThreshold armorRateThreshold `json:"rateLimitThreshold"`
}

type armorRateThreshold struct {
	Count       int `json:"count"`
	IntervalSec int `json:"intervalSec"`
}

type armorWAFConfig struct {
	Exclusions []armorExclusion `json:"exclusions"`
}

type armorExclusion struct {
	TargetRuleSet               string       `json:"targetRuleSet"`
	RequestBodiesToExclude      []armorField `json:"requestBodiesToExclude,omitempty"`
	RequestUrisToExclude        []armorField `json:"requestUrisToExclude,omitempty"`
	RequestQueryParamsToExclude []armorField `json:"requestQueryParamsToExclude,omitempty"`
	RequestCookiesToExclude     []armorField `json:"requestCookiesToExclude,omitempty"`
}

type armorField struct {
	Op string `json:"op"`
}

func equalsAny() []armorField {
	return []armorField{{Op: "EQUALS_ANY"}}
}

// ArmorProtectPolicy returns the Magento-safe Cloud Armor create-from-file document.
func ArmorProtectPolicy(description string) ArmorPolicyDocument {
	armor := Armor()
	doc := ArmorPolicyDocument{
		Description: strings.TrimSpace(description),
		Type:        "CLOUD_ARMOR",
		AdvancedOptionsConfig: armorAdvancedOptions{
			JSONParsing:               armor.JSONParsing,
			LogLevel:                  "VERBOSE",
			RequestBodyInspectionSize: armor.RequestBodyInspection,
		},
		Rules: []armorRule{
			{
				Action:      "allow",
				Priority:    armor.StaticMediaPriority(),
				Description: "allow Magento static and media",
				Match:       armorMatch{Expr: &armorExpr{Expression: armor.StaticMediaCEL}},
			},
			{
				Action:      "throttle",
				Priority:    armor.RateLimitPriority(),
				Description: "Magento-safe IP rate limit",
				Match: armorMatch{
					VersionedExpr: "SRC_IPS_V1",
					Config:        &armorMatchConfig{SrcIPRanges: []string{"*"}},
				},
				RateLimitOptions: &armorRateLimit{
					ConformAction: "allow",
					ExceedAction:  "deny(403)",
					EnforceOnKey:  "IP",
					RateLimitThreshold: armorRateThreshold{
						Count: armor.RateLimitPerIP, IntervalSec: armor.RateLimitWindowSeconds,
					},
				},
			},
		},
	}
	for _, set := range armor.WAFSets {
		rule := armorRule{
			Action:      "deny(403)",
			Priority:    set.Priority,
			Description: set.Name,
			Preview:     set.Preview,
			Match:       armorMatch{Expr: &armorExpr{Expression: ArmorWAFExpression(set.RuleSet)}},
		}
		if exclusion := armorRESTExclusion(set); exclusion != nil {
			rule.PreconfiguredWafConfig = &armorWAFConfig{Exclusions: []armorExclusion{*exclusion}}
		}
		doc.Rules = append(doc.Rules, rule)
	}
	doc.Rules = append(doc.Rules, armorRule{
		Action:      "allow",
		Priority:    armor.DefaultPriority(),
		Description: "default allow",
		Match: armorMatch{
			VersionedExpr: "SRC_IPS_V1",
			Config:        &armorMatchConfig{SrcIPRanges: []string{"*"}},
		},
	})
	return doc
}

func armorRESTExclusion(set ArmorWAFSet) *armorExclusion {
	if !set.ExcludeBody && !set.ExcludeURI && !set.ExcludeQuery && !set.ExcludeCookies {
		return nil
	}
	exclusion := &armorExclusion{TargetRuleSet: set.RuleSet}
	if set.ExcludeBody {
		exclusion.RequestBodiesToExclude = equalsAny()
	}
	if set.ExcludeURI {
		exclusion.RequestUrisToExclude = equalsAny()
	}
	if set.ExcludeQuery {
		exclusion.RequestQueryParamsToExclude = equalsAny()
	}
	if set.ExcludeCookies {
		exclusion.RequestCookiesToExclude = equalsAny()
	}
	return exclusion
}

// MarshalArmorPolicyJSON is the gcloud security-policies create --file-name payload.
func MarshalArmorPolicyJSON(description string) ([]byte, error) {
	return json.Marshal(ArmorProtectPolicy(description))
}
