package edge

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	sdk "github.com/acourtiol/magelift/sdk/v1"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type edgeMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *edgeMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	outputs := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:cloudfront/distribution:Distribution":
		outputs["domainName"] = resource.NewStringProperty("d111111abcdef8.cloudfront.net")
		outputs["hostedZoneId"] = resource.NewStringProperty("Z2FDTNDATAQYW2")
	case "aws:wafv2/webAcl:WebAcl":
		outputs["arn"] = resource.NewStringProperty("arn:aws:wafv2:us-east-1:123456789012:global/webacl/shop/id")
	}
	return args.Name + "-id", outputs, nil
}

func (*edgeMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func TestEdgeResourceGraphEnforcesSecurityBoundaries(t *testing.T) {
	mocks := &edgeMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "global", &awsprovider.ProviderArgs{Region: pulumi.String("us-east-1")})
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop-production", Args{
			DomainName:  "shop.example.com",
			HostedZone:  sdk.ExistingResourceRef{ID: "shop-zone", Provider: "aws", Kind: sdk.ExistingDNSZone, ExternalID: "Z123456789"},
			Certificate: sdk.ExistingResourceRef{ID: "shop-certificate", Provider: "aws", Kind: sdk.ExistingCertificate, ExternalID: "arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000"},
			Origin: ALBOrigin{
				DNSName: pulumi.String("internal-shop-alb.eu-west-3.elb.amazonaws.com"),
				ARN:     "arn:aws:elasticloadbalancing:eu-west-3:123456789012:loadbalancer/app/shop/id",
			},
			GlobalAWS: provider, Security: DefaultSecurityPolicy(),
			Tags: map[string]string{"magelift:managed-by": "magelift"},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}

	mocks.mu.Lock()
	resources := append([]pulumi.MockResourceArgs(nil), mocks.resources...)
	mocks.mu.Unlock()
	byType := make(map[string]pulumi.MockResourceArgs, len(resources))
	for _, registered := range resources {
		byType[registered.TypeToken] = registered
		if strings.Contains(registered.TypeToken, ":ecs/") {
			t.Fatalf("edge component created an ECS resource: %s", registered.TypeToken)
		}
	}
	for _, token := range []string{ComponentToken, "aws:wafv2/webAcl:WebAcl", "aws:cloudfront/distribution:Distribution", "aws:route53/record:Record"} {
		if _, found := byType[token]; !found {
			t.Fatalf("resource graph is missing %s", token)
		}
	}

	distribution := encodedInputs(t, byType["aws:cloudfront/distribution:Distribution"])
	for _, required := range []string{"redirect-to-https", "https-only", "TLSv1.2", "TLSv1.2_2021", "internal-shop-alb.eu-west-3.elb.amazonaws.com"} {
		if !strings.Contains(distribution, required) {
			t.Fatalf("distribution lacks %q: %s", required, distribution)
		}
	}
	if strings.Contains(distribution, "allow-all") {
		t.Fatalf("distribution permits plaintext viewer or origin traffic: %s", distribution)
	}
	waf := encodedInputs(t, byType["aws:wafv2/webAcl:WebAcl"])
	for _, required := range []string{"CLOUDFRONT", "AWSManagedRulesCommonRuleSet", "count"} {
		if !strings.Contains(strings.ToLower(waf), strings.ToLower(required)) {
			t.Fatalf("WAF lacks %q: %s", required, waf)
		}
	}
	record := encodedInputs(t, byType["aws:route53/record:Record"])
	if !strings.Contains(record, "Z123456789") || !strings.Contains(record, "shop.example.com") {
		t.Fatalf("Route 53 alias does not use explicit zone/domain inputs: %s", record)
	}
}

func TestEdgeRejectsMissingOrUnsafeExistingResources(t *testing.T) {
	base := Args{DomainName: "shop.example.com", Origin: ALBOrigin{DNSName: pulumi.String("alb.example.com"), ARN: "arn:aws:elasticloadbalancing:eu-west-3:123456789012:loadbalancer/app/shop/id"}, Security: DefaultSecurityPolicy()}
	tests := []struct {
		name string
		args Args
	}{
		{name: "missing provider", args: base},
		{name: "untyped zone", args: func() Args {
			value := base
			value.HostedZone = sdk.ExistingResourceRef{ID: "zone", Provider: "aws", Kind: sdk.ExistingNetwork, ExternalID: "vpc-1"}
			return value
		}()},
		{name: "regional certificate", args: func() Args {
			value := base
			value.HostedZone = sdk.ExistingResourceRef{ID: "zone", Provider: "aws", Kind: sdk.ExistingDNSZone, ExternalID: "Z1"}
			value.Certificate = sdk.ExistingResourceRef{ID: "certificate", Provider: "aws", Kind: sdk.ExistingCertificate, ExternalID: "arn:aws:acm:eu-west-3:123456789012:certificate/id"}
			return value
		}()},
		{name: "weak TLS", args: func() Args {
			value := base
			value.Security = SecurityPolicy{MinimumTLSVersion: "TLSv1", WAFManagedRules: true, WAFCountMode: true}
			return value
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateArgs("shop", test.args); err == nil {
				t.Fatal("unsafe edge arguments were accepted")
			}
		})
	}
}

func encodedInputs(t *testing.T, args pulumi.MockResourceArgs) string {
	t.Helper()
	encoded, err := json.Marshal(args.Inputs.Mappable())
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
