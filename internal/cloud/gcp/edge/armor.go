package edge

import (
	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type magentoArmorRule struct {
	Name string
	Args compute.SecurityPolicyRuleArgs
}

func magentoArmorRules() []magentoArmorRule {
	armor := waf.Armor()
	rules := []magentoArmorRule{
		{
			Name: "static-media",
			Args: compute.SecurityPolicyRuleArgs{
				Action:      pulumi.String("allow"),
				Priority:    pulumi.Int(armor.StaticMediaPriority()),
				Description: pulumi.String("allow Magento static and media"),
				Match: &compute.SecurityPolicyRuleMatchArgs{
					Expr: &compute.SecurityPolicyRuleMatchExprArgs{Expression: pulumi.String(armor.StaticMediaCEL)},
				},
			},
		},
		{
			Name: "rate-limit",
			Args: compute.SecurityPolicyRuleArgs{
				Action:      pulumi.String("throttle"),
				Priority:    pulumi.Int(armor.RateLimitPriority()),
				Description: pulumi.String("Magento-safe IP rate limit"),
				Match: &compute.SecurityPolicyRuleMatchArgs{
					VersionedExpr: pulumi.String("SRC_IPS_V1"),
					Config:        &compute.SecurityPolicyRuleMatchConfigArgs{SrcIpRanges: pulumi.StringArray{pulumi.String("*")}},
				},
				RateLimitOptions: &compute.SecurityPolicyRuleRateLimitOptionsArgs{
					ConformAction: pulumi.String("allow"),
					ExceedAction:  pulumi.String("deny(403)"),
					EnforceOnKey:  pulumi.String("IP"),
					RateLimitThreshold: &compute.SecurityPolicyRuleRateLimitOptionsRateLimitThresholdArgs{
						Count: pulumi.Int(armor.RateLimitPerIP), IntervalSec: pulumi.Int(armor.RateLimitWindowSeconds),
					},
				},
			},
		},
	}
	for _, set := range armor.WAFSets {
		rule := magentoArmorRule{
			Name: set.Name,
			Args: compute.SecurityPolicyRuleArgs{
				Action:      pulumi.String("deny(403)"),
				Priority:    pulumi.Int(set.Priority),
				Description: pulumi.String(set.Name),
				Match: &compute.SecurityPolicyRuleMatchArgs{
					Expr: &compute.SecurityPolicyRuleMatchExprArgs{Expression: pulumi.String(waf.ArmorWAFExpression(set.RuleSet))},
				},
			},
		}
		if set.Preview {
			rule.Args.Preview = pulumi.Bool(true)
		}
		if exclusion := armorExclusion(set); exclusion != nil {
			rule.Args.PreconfiguredWafConfig = &compute.SecurityPolicyRulePreconfiguredWafConfigArgs{
				Exclusions: compute.SecurityPolicyRulePreconfiguredWafConfigExclusionArray{exclusion},
			}
		}
		rules = append(rules, rule)
	}
	rules = append(rules, magentoArmorRule{
		Name: "default",
		Args: compute.SecurityPolicyRuleArgs{
			Action:      pulumi.String("allow"),
			Priority:    pulumi.Int(armor.DefaultPriority()),
			Description: pulumi.String("default allow"),
			Match: &compute.SecurityPolicyRuleMatchArgs{
				VersionedExpr: pulumi.String("SRC_IPS_V1"),
				Config:        &compute.SecurityPolicyRuleMatchConfigArgs{SrcIpRanges: pulumi.StringArray{pulumi.String("*")}},
			},
		},
	})
	return rules
}

func magentoArmorAdvancedOptions() *compute.SecurityPolicyAdvancedOptionsConfigArgs {
	armor := waf.Armor()
	return &compute.SecurityPolicyAdvancedOptionsConfigArgs{
		JsonParsing:               pulumi.String(armor.JSONParsing),
		LogLevel:                  pulumi.String("VERBOSE"),
		RequestBodyInspectionSize: pulumi.String(armor.RequestBodyInspection),
	}
}

func armorExclusion(set waf.ArmorWAFSet) *compute.SecurityPolicyRulePreconfiguredWafConfigExclusionArgs {
	if !set.ExcludeBody && !set.ExcludeURI && !set.ExcludeQuery && !set.ExcludeCookies {
		return nil
	}
	exclusion := &compute.SecurityPolicyRulePreconfiguredWafConfigExclusionArgs{TargetRuleSet: pulumi.String(set.RuleSet)}
	if set.ExcludeBody {
		exclusion.RequestBodies = compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestBodyArray{
			&compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestBodyArgs{Operator: pulumi.String("EQUALS_ANY")},
		}
	}
	if set.ExcludeURI {
		exclusion.RequestUris = compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestUriArray{
			&compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestUriArgs{Operator: pulumi.String("EQUALS_ANY")},
		}
	}
	if set.ExcludeQuery {
		exclusion.RequestQueryParams = compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestQueryParamArray{
			&compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestQueryParamArgs{Operator: pulumi.String("EQUALS_ANY")},
		}
	}
	if set.ExcludeCookies {
		exclusion.RequestCookies = compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestCookyArray{
			&compute.SecurityPolicyRulePreconfiguredWafConfigExclusionRequestCookyArgs{Operator: pulumi.String("EQUALS_ANY")},
		}
	}
	return exclusion
}
