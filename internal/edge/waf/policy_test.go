package waf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMagentoSafeForUsesConfiguredFrontName(t *testing.T) {
	if MagentoSafe().AdminPathPrefix != "/admin" {
		t.Fatalf("default admin path = %q", MagentoSafe().AdminPathPrefix)
	}
	policy := MagentoSafeFor("backend")
	if policy.AdminPathPrefix != "/backend" {
		t.Fatalf("configured admin path = %q", policy.AdminPathPrefix)
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"/admin"`) {
		t.Fatalf("configured Magento-safe policy still hardcodes /admin: %s", encoded)
	}
	unmodified := `{"Name":"AWSManagedRulesCommonRuleSet"}`
	if !strings.Contains(string(encoded), "SizeRestrictions_BODY") || strings.Contains(string(encoded), unmodified) {
		t.Fatalf("policy must count-override Magento CRS false positives, not attach unmodified CRS: %s", encoded)
	}
}

func TestMagentoSafeCountsLivedAWSFalsePositives(t *testing.T) {
	policy := MagentoSafe()
	if policy.RateLimitPerIP != 2000 || policy.BodyInspectionKB != 64 || policy.ScalewayMaxParanoia != 1 {
		t.Fatalf("Magento-safe scalars = %#v", policy)
	}
	required := []string{
		"SizeRestrictions_BODY", "CrossSiteScripting_BODY", "GenericRFI_BODY",
		"GenericLFI_BODY", "EC2MetaDataSSRF_BODY", "NoUserAgent_HEADER",
		"Log4JRCE_BODY", "SQLi_BODY", "SQLiExtendedPatterns_QUERYARGUMENTS",
		"PHPHighRiskMethodsVariables_BODY", "LFI_URIPATH", "LFI_QUERYSTRING",
	}
	found := map[string]bool{}
	groups := map[string]bool{}
	for _, group := range policy.AWSManagedGroups {
		groups[group.Name] = true
		for _, name := range group.CountOverrides {
			found[name] = true
		}
	}
	for _, name := range required {
		if !found[name] {
			t.Fatalf("missing Magento AWS count-override %s", name)
		}
	}
	for _, name := range []string{
		"AWSManagedRulesAmazonIpReputationList", "AWSManagedRulesCommonRuleSet",
		"AWSManagedRulesKnownBadInputsRuleSet", "AWSManagedRulesSQLiRuleSet",
		"AWSManagedRulesPHPRuleSet", "AWSManagedRulesLinuxRuleSet",
	} {
		if !groups[name] {
			t.Fatalf("missing Magento AWS managed group %s", name)
		}
	}
}

func TestAWSProtectWebACLBlocksGroupsAndCountsMagentoRules(t *testing.T) {
	rules, err := MarshalAWSRulesJSON("magelift-cf-waf-test")
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(rules)
	if strings.Contains(encoded, `"OverrideAction":{"Count"`) {
		t.Fatalf("Magento-safe ACL must not count entire managed groups: %s", encoded)
	}
	for _, required := range []string{
		`"None":{}`, "SizeRestrictions_BODY", "SQLi_BODY", "AWSManagedRulesSQLiRuleSet",
		`"Limit":2000`, `"AggregateKeyType":"IP"`,
	} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("AWS rules lack %s: %s", required, encoded)
		}
	}
	assoc, err := MarshalAWSAssociationJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(assoc), `"DefaultSizeInspectionLimit":"KB_64"`) || !strings.Contains(string(assoc), `"CLOUDFRONT"`) || strings.Contains(string(assoc), `"CloudFront"`) {
		t.Fatalf("association config = %s", assoc)
	}
	var parsed []map[string]any
	if err := json.Unmarshal(rules, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1+len(MagentoSafe().AWSManagedGroups) {
		t.Fatalf("rule count = %d", len(parsed))
	}
}

func TestArmorMapsMagentoExclusionsAndKeepsScannerBlock(t *testing.T) {
	armor := Armor()
	if armor.JSONParsing != "STANDARD_WITH_GRAPHQL" || armor.RequestBodyInspection != "64KB" {
		t.Fatalf("armor inspection = %#v", armor)
	}
	if !strings.Contains(armor.StaticMediaCEL, "/static/") || !strings.Contains(armor.StaticMediaCEL, "/media/") {
		t.Fatalf("static/media CEL = %s", armor.StaticMediaCEL)
	}
	var sawSQLi, sawScanner bool
	for _, set := range armor.WAFSets {
		if set.RuleSet == "sqli-v33-stable" {
			sawSQLi = true
			if !set.ExcludeBody || !set.ExcludeQuery || set.Preview {
				t.Fatalf("sqli Magento mapping = %#v", set)
			}
		}
		if set.RuleSet == "scannerdetection-v33-stable" {
			sawScanner = true
			if set.Preview || set.ExcludeBody {
				t.Fatalf("scannerdetection must stay blocking: %#v", set)
			}
		}
	}
	if !sawSQLi || !sawScanner {
		t.Fatalf("armor WAF sets = %#v", armor.WAFSets)
	}
	if got := ArmorWAFExpression("sqli-v33-stable"); !strings.Contains(got, "sensitivity") || !strings.Contains(got, "sqli-v33-stable") {
		t.Fatalf("armor expression = %s", got)
	}
	payload, err := MarshalArmorPolicyJSON("magelift ownership=test")
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, required := range []string{
		`"type":"CLOUD_ARMOR"`, `"jsonParsing":"STANDARD_WITH_GRAPHQL"`, `"requestBodyInspectionSize":"64KB"`,
		"requestBodiesToExclude", `"op":"EQUALS_ANY"`, "scannerdetection-v33-stable",
		`'sensitivity': 1`, `"count":2000`, `"intervalSec":300`,
	} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("Armor JSON lacks %s: %s", required, encoded)
		}
	}
	if strings.Contains(encoded, `"preview":true`) && !strings.Contains(encoded, "sessionfixation") {
		t.Fatalf("unexpected preview-only policy: %s", encoded)
	}
}

func TestFastlyExclusionsSkipStaticMediaNotAdmin(t *testing.T) {
	fastly := Fastly()
	if fastly.PolicyRef != PolicyRef || fastly.RateLimitPerIP != RateLimitPerIP {
		t.Fatalf("Fastly NGWAF translation = %#v", fastly)
	}
	if len(fastly.PathExclusions) == 0 {
		t.Fatal("Fastly Magento-safe translation needs static/media exclusions")
	}
	for _, exclusion := range fastly.PathExclusions {
		if strings.Contains(exclusion.PathPrefix, "admin") || strings.Contains(exclusion.PathPrefix, "graphql") || strings.Contains(exclusion.PathPrefix, "rest") {
			t.Fatalf("admin/API must stay in WAF scope: %#v", exclusion)
		}
	}
}
