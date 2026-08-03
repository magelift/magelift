package edge

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudfront"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/wafv2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:Edge"

var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

type SecurityPolicy struct {
	MinimumTLSVersion string
	WAFManagedRules   bool
	WAFCountMode      bool
}

func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{MinimumTLSVersion: "TLSv1.2_2021", WAFManagedRules: true, WAFCountMode: true}
}

type ALBOrigin struct {
	DNSName  pulumi.StringInput
	ARN      string
	ARNInput pulumi.StringInput
}

type Args struct {
	DomainName  string
	HostedZone  sdk.ExistingResourceRef
	Certificate sdk.ExistingResourceRef
	Origin      ALBOrigin
	GlobalAWS   *awsprovider.Provider
	Security    SecurityPolicy
	Tags        map[string]string
}

type Component struct {
	pulumi.ResourceState
	DistributionID         pulumi.IDOutput     `pulumi:"distributionId"`
	DistributionDomainName pulumi.StringOutput `pulumi:"distributionDomainName"`
	WebACLARN              pulumi.StringOutput `pulumi:"webAclArn"`
}

func New(ctx *pulumi.Context, name string, args Args, options ...pulumi.ResourceOption) (*Component, error) {
	if err := validateArgs(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, options...); err != nil {
		return nil, err
	}
	childOptions := []pulumi.ResourceOption{pulumi.Parent(component), pulumi.Provider(args.GlobalAWS)}

	// Managed CachingDisabled (TTL 0). Custom cache policies with caching disabled
	// reject cookie/query cache-key behaviors and Accept-Encoding toggles; viewer
	// cookies and query strings are still forwarded via OriginRequestPolicy.
	const cachingDisabledPolicyID = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
	originRequestPolicy, err := cloudfront.NewOriginRequestPolicy(ctx, name+"-origin", &cloudfront.OriginRequestPolicyArgs{
		Name: pulumi.String(name + "-origin"), Comment: pulumi.String("Forward Magento request context to the ALB origin"),
		CookiesConfig:      &cloudfront.OriginRequestPolicyCookiesConfigArgs{CookieBehavior: pulumi.String("all")},
		HeadersConfig:      &cloudfront.OriginRequestPolicyHeadersConfigArgs{HeaderBehavior: pulumi.String("allViewer")},
		QueryStringsConfig: &cloudfront.OriginRequestPolicyQueryStringsConfigArgs{QueryStringBehavior: pulumi.String("all")},
	}, childOptions...)
	if err != nil {
		return nil, fmt.Errorf("create CloudFront origin request policy: %w", err)
	}

	webACL, err := wafv2.NewWebAcl(ctx, name, &wafv2.WebAclArgs{
		Name: pulumi.String(name), Scope: pulumi.String("CLOUDFRONT"), Region: pulumi.String("us-east-1"),
		DefaultAction: &wafv2.WebAclDefaultActionArgs{Allow: &wafv2.WebAclDefaultActionAllowArgs{}},
		Rules: wafv2.WebAclRuleTypeArray{&wafv2.WebAclRuleTypeArgs{
			Name: pulumi.String("aws-common"), Priority: pulumi.Int(0),
			OverrideAction:   &wafv2.WebAclRuleOverrideActionArgs{Count: &wafv2.WebAclRuleOverrideActionCountArgs{}},
			Statement:        &wafv2.WebAclRuleStatementArgs{ManagedRuleGroupStatement: &wafv2.WebAclRuleStatementManagedRuleGroupStatementArgs{Name: pulumi.String("AWSManagedRulesCommonRuleSet"), VendorName: pulumi.String("AWS")}},
			VisibilityConfig: &wafv2.WebAclRuleVisibilityConfigArgs{CloudwatchMetricsEnabled: pulumi.Bool(true), MetricName: pulumi.String(name + "-aws-common"), SampledRequestsEnabled: pulumi.Bool(true)},
		}},
		VisibilityConfig: &wafv2.WebAclVisibilityConfigArgs{CloudwatchMetricsEnabled: pulumi.Bool(true), MetricName: pulumi.String(name), SampledRequestsEnabled: pulumi.Bool(true)},
		Tags:             pulumi.ToStringMap(args.Tags),
	}, childOptions...)
	if err != nil {
		return nil, fmt.Errorf("create WAF WebACL: %w", err)
	}

	distribution, err := cloudfront.NewDistribution(ctx, name, &cloudfront.DistributionArgs{
		Aliases: pulumi.StringArray{pulumi.String(args.DomainName)}, Enabled: pulumi.Bool(true), IsIpv6Enabled: pulumi.Bool(true), HttpVersion: pulumi.String("http2and3"),
		Origins: cloudfront.DistributionOriginArray{&cloudfront.DistributionOriginArgs{
			DomainName: args.Origin.DNSName, OriginId: pulumi.String("alb"),
			CustomOriginConfig: &cloudfront.DistributionOriginCustomOriginConfigArgs{HttpPort: pulumi.Int(80), HttpsPort: pulumi.Int(443), OriginProtocolPolicy: pulumi.String("https-only"), OriginSslProtocols: pulumi.StringArray{pulumi.String("TLSv1.2")}},
		}},
		DefaultCacheBehavior: &cloudfront.DistributionDefaultCacheBehaviorArgs{
			AllowedMethods: pulumi.ToStringArray([]string{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}), CachedMethods: pulumi.ToStringArray([]string{"GET", "HEAD", "OPTIONS"}),
			TargetOriginId: pulumi.String("alb"), ViewerProtocolPolicy: pulumi.String("redirect-to-https"), Compress: pulumi.Bool(true), CachePolicyId: pulumi.String(cachingDisabledPolicyID), OriginRequestPolicyId: originRequestPolicy.ID(),
		},
		Restrictions:      &cloudfront.DistributionRestrictionsArgs{GeoRestriction: &cloudfront.DistributionRestrictionsGeoRestrictionArgs{RestrictionType: pulumi.String("none")}},
		ViewerCertificate: &cloudfront.DistributionViewerCertificateArgs{AcmCertificateArn: pulumi.String(args.Certificate.ExternalID), MinimumProtocolVersion: pulumi.String(args.Security.MinimumTLSVersion), SslSupportMethod: pulumi.String("sni-only")},
		WebAclId:          webACL.Arn, Tags: pulumi.ToStringMap(args.Tags),
	}, childOptions...)
	if err != nil {
		return nil, fmt.Errorf("create CloudFront distribution: %w", err)
	}

	_, err = route53.NewRecord(ctx, name, &route53.RecordArgs{
		ZoneId: pulumi.String(args.HostedZone.ExternalID), Name: pulumi.String(args.DomainName), Type: pulumi.String("A"),
		Aliases: route53.RecordAliasArray{&route53.RecordAliasArgs{Name: distribution.DomainName, ZoneId: distribution.HostedZoneId, EvaluateTargetHealth: pulumi.Bool(false)}},
	}, childOptions...)
	if err != nil {
		return nil, fmt.Errorf("create Route 53 alias: %w", err)
	}

	component.DistributionID = distribution.ID()
	component.DistributionDomainName = distribution.DomainName
	component.WebACLARN = webACL.Arn
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"distributionId": distribution.ID(), "distributionDomainName": distribution.DomainName, "webAclArn": webACL.Arn}); err != nil {
		return nil, err
	}
	return component, nil
}

func validateArgs(name string, args Args) error {
	if strings.TrimSpace(name) == "" || !domainPattern.MatchString(args.DomainName) {
		return errors.New("edge name and domain are required")
	}
	validARN := args.Origin.ARNInput != nil || (strings.HasPrefix(args.Origin.ARN, "arn:aws:elasticloadbalancing:") && strings.Contains(args.Origin.ARN, ":loadbalancer/app/"))
	if args.GlobalAWS == nil || args.Origin.DNSName == nil || !validARN {
		return errors.New("edge global AWS provider and ALB origin are required")
	}
	if err := sdk.ValidateExistingResourceRef(args.HostedZone); err != nil || args.HostedZone.Provider != "aws" || args.HostedZone.Kind != sdk.ExistingDNSZone {
		return errors.New("edge hosted zone must be an explicit AWS DNS zone reference")
	}
	if err := sdk.ValidateExistingResourceRef(args.Certificate); err != nil || args.Certificate.Provider != "aws" || args.Certificate.Kind != sdk.ExistingCertificate || !strings.HasPrefix(args.Certificate.ExternalID, "arn:aws:acm:us-east-1:") {
		return errors.New("edge certificate must be an explicit us-east-1 AWS certificate reference")
	}
	if args.Security.MinimumTLSVersion != "TLSv1.2_2021" || !args.Security.WAFManagedRules || !args.Security.WAFCountMode {
		return errors.New("edge security policy must enforce TLS 1.2 and managed WAF rules in count mode")
	}
	return nil
}
