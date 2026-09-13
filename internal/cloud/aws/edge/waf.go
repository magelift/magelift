package edge

import (
	"strings"

	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/wafv2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func magentoWebACLArgs(name string, countMode bool, frontName string, tags map[string]string) *wafv2.WebAclArgs {
	policy := waf.MagentoSafeFor(frontName)
	if tags == nil {
		tags = map[string]string{}
	}
	tagged := make(map[string]string, len(tags)+1)
	for key, value := range tags {
		tagged[key] = value
	}
	tagged["magelift:magento-front-name"] = strings.TrimPrefix(policy.AdminPathPrefix, "/")
	rules := wafv2.WebAclRuleTypeArray{
		&wafv2.WebAclRuleTypeArgs{
			Name:     pulumi.String("RateLimit"),
			Priority: pulumi.Int(1),
			Action:   &wafv2.WebAclRuleActionArgs{Block: &wafv2.WebAclRuleActionBlockArgs{}},
			Statement: &wafv2.WebAclRuleStatementArgs{RateBasedStatement: &wafv2.WebAclRuleStatementRateBasedStatementArgs{
				Limit: pulumi.Int(policy.RateLimitPerIP), AggregateKeyType: pulumi.String("IP"),
			}},
			VisibilityConfig: &wafv2.WebAclRuleVisibilityConfigArgs{CloudwatchMetricsEnabled: pulumi.Bool(true), MetricName: pulumi.String(name + "-RateLimit"), SampledRequestsEnabled: pulumi.Bool(true)},
		},
		&wafv2.WebAclRuleTypeArgs{
			Name:     pulumi.String("MagentoAdminFrontName"),
			Priority: pulumi.Int(8),
			Action:   &wafv2.WebAclRuleActionArgs{Count: &wafv2.WebAclRuleActionCountArgs{}},
			Statement: &wafv2.WebAclRuleStatementArgs{ByteMatchStatement: &wafv2.WebAclRuleStatementByteMatchStatementArgs{
				SearchString:         pulumi.String(policy.AdminPathPrefix),
				PositionalConstraint: pulumi.String("STARTS_WITH"),
				FieldToMatch:         &wafv2.WebAclRuleStatementByteMatchStatementFieldToMatchArgs{UriPath: &wafv2.WebAclRuleStatementByteMatchStatementFieldToMatchUriPathArgs{}},
				TextTransformations: wafv2.WebAclRuleStatementByteMatchStatementTextTransformationArray{
					&wafv2.WebAclRuleStatementByteMatchStatementTextTransformationArgs{Priority: pulumi.Int(0), Type: pulumi.String("NONE")},
				},
			}},
			VisibilityConfig: &wafv2.WebAclRuleVisibilityConfigArgs{CloudwatchMetricsEnabled: pulumi.Bool(true), MetricName: pulumi.String(name + "-MagentoAdmin"), SampledRequestsEnabled: pulumi.Bool(true)},
		},
	}
	for _, group := range policy.AWSManagedGroups {
		override := &wafv2.WebAclRuleOverrideActionArgs{None: &wafv2.WebAclRuleOverrideActionNoneArgs{}}
		if countMode {
			override = &wafv2.WebAclRuleOverrideActionArgs{Count: &wafv2.WebAclRuleOverrideActionCountArgs{}}
		}
		managed := &wafv2.WebAclRuleStatementManagedRuleGroupStatementArgs{
			Name: pulumi.String(group.Name), VendorName: pulumi.String("AWS"),
		}
		if len(group.CountOverrides) > 0 {
			overrides := make(wafv2.WebAclRuleStatementManagedRuleGroupStatementRuleActionOverrideArray, 0, len(group.CountOverrides))
			for _, ruleName := range group.CountOverrides {
				overrides = append(overrides, &wafv2.WebAclRuleStatementManagedRuleGroupStatementRuleActionOverrideArgs{
					Name: pulumi.String(ruleName),
					ActionToUse: &wafv2.WebAclRuleStatementManagedRuleGroupStatementRuleActionOverrideActionToUseArgs{
						Count: &wafv2.WebAclRuleStatementManagedRuleGroupStatementRuleActionOverrideActionToUseCountArgs{},
					},
				})
			}
			managed.RuleActionOverrides = overrides
		}
		rules = append(rules, &wafv2.WebAclRuleTypeArgs{
			Name: pulumi.String(group.Name), Priority: pulumi.Int(group.Priority),
			OverrideAction:   override,
			Statement:        &wafv2.WebAclRuleStatementArgs{ManagedRuleGroupStatement: managed},
			VisibilityConfig: &wafv2.WebAclRuleVisibilityConfigArgs{CloudwatchMetricsEnabled: pulumi.Bool(true), MetricName: pulumi.String(group.Name), SampledRequestsEnabled: pulumi.Bool(true)},
		})
	}
	return &wafv2.WebAclArgs{
		Name: pulumi.String(name), Scope: pulumi.String("CLOUDFRONT"), Region: pulumi.String("us-east-1"),
		DefaultAction: &wafv2.WebAclDefaultActionArgs{Allow: &wafv2.WebAclDefaultActionAllowArgs{}},
		AssociationConfig: &wafv2.WebAclAssociationConfigArgs{
			RequestBodies: wafv2.WebAclAssociationConfigRequestBodyArray{
				wafv2.WebAclAssociationConfigRequestBodyArgs{
					Cloudfront: &wafv2.WebAclAssociationConfigRequestBodyCloudfrontArgs{DefaultSizeInspectionLimit: pulumi.String("KB_64")},
				},
			},
		},
		Rules:            rules,
		VisibilityConfig: &wafv2.WebAclVisibilityConfigArgs{CloudwatchMetricsEnabled: pulumi.Bool(true), MetricName: pulumi.String(name), SampledRequestsEnabled: pulumi.Bool(true)},
		Tags:             pulumi.ToStringMap(tagged),
	}
}
