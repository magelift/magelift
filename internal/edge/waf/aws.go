package waf

import (
	"encoding/json"
	"fmt"
	"strings"
)

const awsBodyInspectionLimit = "KB_64"

// AWSWebACLDocuments is the AWS CLI create-web-acl payload for Magento-safe
// protect mode: managed groups block except Magento count-overrides, plus a
// per-IP rate limit and 64 KB CloudFront body inspection.
type AWSWebACLDocuments struct {
	Rules             []awsRule        `json:"Rules"`
	AssociationConfig awsAssociation   `json:"AssociationConfig"`
	DefaultAction     awsDefaultAction `json:"DefaultAction"`
	Scope             string           `json:"Scope"`
}

type awsDefaultAction struct {
	Allow struct{} `json:"Allow"`
}

type awsAssociation struct {
	RequestBody awsAssociationRequestBody `json:"RequestBody"`
}

type awsAssociationRequestBody struct {
	CloudFront awsCloudFrontBody `json:"CLOUDFRONT"`
}

type awsCloudFrontBody struct {
	DefaultSizeInspectionLimit string `json:"DefaultSizeInspectionLimit"`
}

type awsRule struct {
	Name             string             `json:"Name"`
	Priority         int                `json:"Priority"`
	Action           *awsAction         `json:"Action,omitempty"`
	OverrideAction   *awsOverrideAction `json:"OverrideAction,omitempty"`
	Statement        awsStatement       `json:"Statement"`
	VisibilityConfig awsVisibility      `json:"VisibilityConfig"`
}

type awsAction struct {
	Block struct{} `json:"Block"`
}

type awsOverrideAction struct {
	None  *struct{} `json:"None,omitempty"`
	Count *struct{} `json:"Count,omitempty"`
}

type awsStatement struct {
	RateBasedStatement        *awsRateStatement    `json:"RateBasedStatement,omitempty"`
	ManagedRuleGroupStatement *awsManagedStatement `json:"ManagedRuleGroupStatement,omitempty"`
}

type awsRateStatement struct {
	Limit            int    `json:"Limit"`
	AggregateKeyType string `json:"AggregateKeyType"`
}

type awsManagedStatement struct {
	VendorName          string            `json:"VendorName"`
	Name                string            `json:"Name"`
	RuleActionOverrides []awsRuleOverride `json:"RuleActionOverrides,omitempty"`
}

type awsRuleOverride struct {
	Name        string         `json:"Name"`
	ActionToUse awsOverrideUse `json:"ActionToUse"`
}

type awsOverrideUse struct {
	Count struct{} `json:"Count"`
}

type awsVisibility struct {
	SampledRequestsEnabled   bool   `json:"SampledRequestsEnabled"`
	CloudWatchMetricsEnabled bool   `json:"CloudWatchMetricsEnabled"`
	MetricName               string `json:"MetricName"`
}

// AWSProtectWebACL returns the Magento-safe CLOUDFRONT-scope WebACL documents.
// metricPrefix is used only for the rate-limit metric; managed groups keep
// their AWS names so CloudWatch metrics match production.
func AWSProtectWebACL(metricPrefix string) (AWSWebACLDocuments, error) {
	prefix := strings.TrimSpace(metricPrefix)
	if prefix == "" {
		return AWSWebACLDocuments{}, fmt.Errorf("WAF metric prefix is required")
	}
	policy := MagentoSafe()
	none := struct{}{}
	docs := AWSWebACLDocuments{
		Scope:             "CLOUDFRONT",
		DefaultAction:     awsDefaultAction{},
		AssociationConfig: awsAssociation{RequestBody: awsAssociationRequestBody{CloudFront: awsCloudFrontBody{DefaultSizeInspectionLimit: awsBodyInspectionLimit}}},
		Rules: []awsRule{{
			Name:             "RateLimit",
			Priority:         1,
			Action:           &awsAction{},
			Statement:        awsStatement{RateBasedStatement: &awsRateStatement{Limit: policy.RateLimitPerIP, AggregateKeyType: "IP"}},
			VisibilityConfig: awsVisibility{SampledRequestsEnabled: true, CloudWatchMetricsEnabled: true, MetricName: prefix + "-RateLimit"},
		}},
	}
	for _, group := range policy.AWSManagedGroups {
		rule := awsRule{
			Name:           group.Name,
			Priority:       group.Priority,
			OverrideAction: &awsOverrideAction{None: &none},
			Statement: awsStatement{ManagedRuleGroupStatement: &awsManagedStatement{
				VendorName: "AWS",
				Name:       group.Name,
			}},
			VisibilityConfig: awsVisibility{SampledRequestsEnabled: true, CloudWatchMetricsEnabled: true, MetricName: group.Name},
		}
		if len(group.CountOverrides) > 0 {
			overrides := make([]awsRuleOverride, 0, len(group.CountOverrides))
			for _, name := range group.CountOverrides {
				overrides = append(overrides, awsRuleOverride{Name: name})
			}
			rule.Statement.ManagedRuleGroupStatement.RuleActionOverrides = overrides
		}
		docs.Rules = append(docs.Rules, rule)
	}
	return docs, nil
}

// MarshalAWSRulesJSON is the AWS CLI --rules file contents.
func MarshalAWSRulesJSON(metricPrefix string) ([]byte, error) {
	docs, err := AWSProtectWebACL(metricPrefix)
	if err != nil {
		return nil, err
	}
	return json.Marshal(docs.Rules)
}

// MarshalAWSAssociationJSON is the AWS CLI --association-config file contents.
func MarshalAWSAssociationJSON() ([]byte, error) {
	docs, err := AWSProtectWebACL("magelift-waf")
	if err != nil {
		return nil, err
	}
	return json.Marshal(docs.AssociationConfig)
}
