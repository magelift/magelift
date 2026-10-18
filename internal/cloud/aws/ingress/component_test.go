package ingress

import (
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/internal/cloud/aws/naming"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type node struct {
	typeToken string
	name      string
	inputs    resource.PropertyMap
}

type mocks struct {
	mu    sync.Mutex
	nodes []node
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.nodes = append(m.nodes, node{typeToken: args.TypeToken, name: args.Name, inputs: args.Inputs.Copy()})
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:lb/loadBalancer:LoadBalancer":
		state["arn"] = resource.NewStringProperty("arn:aws:elasticloadbalancing:eu-west-3:123456789012:loadbalancer/app/shop/abc")
		state["arnSuffix"] = resource.NewStringProperty("app/shop/abc")
		state["dnsName"] = resource.NewStringProperty("shop.eu-west-3.elb.amazonaws.com")
	case "aws:lb/targetGroup:TargetGroup":
		state["arn"] = resource.NewStringProperty("arn:aws:elasticloadbalancing:eu-west-3:123456789012:targetgroup/shop/def")
	case "aws:lb/listener:Listener":
		state["arn"] = resource.NewStringProperty("arn:aws:elasticloadbalancing:eu-west-3:123456789012:listener/app/shop/abc/ghi")
	}
	return args.Name + "-id", state, nil
}

func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestCreatesSecureInternetFacingHTTPSIngress(t *testing.T) {
	t.Parallel()
	m := deploy(t, validArgs())
	want := []string{
		"aws:lb/listener:Listener:shop-https",
		"aws:lb/loadBalancer:LoadBalancer:shop-alb",
		"aws:lb/targetGroup:TargetGroup:shop-web",
		TypeToken + ":shop",
		"pulumi:providers:aws:regional",
	}
	if got := m.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("resource graph:\n%s", strings.Join(got, "\n"))
	}
	loadBalancer := m.one(t, "aws:lb/loadBalancer:LoadBalancer").inputs
	if loadBalancer["internal"].BoolValue() || loadBalancer["loadBalancerType"].StringValue() != "application" || !loadBalancer["dropInvalidHeaderFields"].BoolValue() {
		t.Fatalf("load balancer inputs = %#v", loadBalancer.Mappable())
	}
	if loadBalancer["name"].StringValue() != "shop-alb" {
		t.Fatalf("load balancer name = %q", loadBalancer["name"].StringValue())
	}
	if len(loadBalancer["subnets"].ArrayValue()) != 2 || len(loadBalancer["securityGroups"].ArrayValue()) != 1 {
		t.Fatal("load balancer placement is incomplete")
	}
	target := m.one(t, "aws:lb/targetGroup:TargetGroup").inputs
	if target["name"].StringValue() != "shop-web" {
		t.Fatalf("target group name = %q", target["name"].StringValue())
	}
	if target["targetType"].StringValue() != "ip" || target["protocol"].StringValue() != "HTTP" || target["port"].NumberValue() != 8080 {
		t.Fatalf("target group inputs = %#v", target.Mappable())
	}
	listener := m.one(t, "aws:lb/listener:Listener").inputs
	if listener["protocol"].StringValue() != "HTTPS" || listener["port"].NumberValue() != 443 || listener["sslPolicy"].StringValue() != "ELBSecurityPolicy-TLS13-1-2-2021-06" {
		t.Fatalf("listener inputs = %#v", listener.Mappable())
	}
}

func TestExposesIngressOutputs(t *testing.T) {
	t.Parallel()
	m := &mocks{}
	var outputs []string
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
		if err != nil {
			return err
		}
		args := validArgs()
		args.Provider = provider
		component, err := New(ctx, "shop", args)
		if err != nil {
			return err
		}
		pulumi.All(component.LoadBalancerARN, component.DNSName, component.LoadBalancerDimension, component.TargetGroupARN, component.ListenerARN).ApplyT(func(values []any) error {
			for _, value := range values {
				outputs = append(outputs, value.(string))
			}
			return nil
		})
		return nil
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 5 || outputs[2] != "app/shop/abc" {
		t.Fatalf("outputs = %#v", outputs)
	}
}

func TestRejectsInvalidInputsBeforeIngressRegistration(t *testing.T) {
	t.Parallel()
	tests := []func(*Args){
		func(args *Args) { args.CertificateARN = pulumi.String("certificate/latest") },
		func(args *Args) { args.CertificateARN = nil },
		func(args *Args) { args.PublicSubnetIDs = args.PublicSubnetIDs[:1] },
		func(args *Args) { args.PublicSubnetIDs[1] = args.PublicSubnetIDs[0] },
		func(args *Args) { args.VPCID = nil },
		func(args *Args) { args.SecurityGroupID = nil },
		func(args *Args) { args.HealthCheckPath = "health" },
	}
	for index, mutate := range tests {
		args := validArgs()
		mutate(&args)
		m := &mocks{}
		err := pulumi.RunErr(func(ctx *pulumi.Context) error {
			provider, providerErr := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
			if providerErr != nil {
				return providerErr
			}
			args.Provider = provider
			_, newErr := New(ctx, "shop", args)
			return newErr
		}, pulumi.WithMocks("magelift", "test", m))
		if err == nil {
			t.Fatalf("case %d was accepted", index)
		}
		if got := m.countWithoutProvider(); got != 0 {
			t.Fatalf("case %d registered %d ingress resources", index, got)
		}
	}
}

func validArgs() Args {
	return Args{
		Region: "eu-west-3", VPCID: pulumi.String("vpc-123"),
		PublicSubnetIDs: pulumi.StringArray{pulumi.String("subnet-public-a"), pulumi.String("subnet-public-b")},
		SecurityGroupID: pulumi.String("sg-alb"), CertificateARN: pulumi.String("arn:aws:acm:eu-west-3:123456789012:certificate/00000000-0000-0000-0000-000000000000"),
		TargetPort: 8080, HealthCheckPath: "/health", Tags: map[string]string{"magelift:managed-by": "magelift"},
	}
}

func deploy(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
		if err != nil {
			return err
		}
		args.Provider = provider
		_, err = New(ctx, "shop", args)
		return err
	}, pulumi.WithMocks("magelift", "test", m)); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *mocks) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.nodes))
	for index, n := range m.nodes {
		result[index] = n.typeToken + ":" + n.name
	}
	sort.Strings(result)
	return result
}

func (m *mocks) one(t *testing.T, token string) node {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.typeToken == token {
			return n
		}
	}
	t.Fatalf("resource %s not found", token)
	return node{}
}

func (m *mocks) countWithoutProvider() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.nodes {
		if n.typeToken != "pulumi:providers:aws" {
			count++
		}
	}
	return count
}

func TestAWSResourceNameRespectsLimit(t *testing.T) {
	t.Parallel()
	exact := "acceptance-preview-ingress-alb" // 32 characters
	if got := naming.AWSName(exact, 32); got != exact {
		t.Fatalf("exact-limit name changed: %q", got)
	}
	long := exact + "-extra"
	got := naming.AWSName(long, 32)
	if len(got) > 32 {
		t.Fatalf("name too long: %q (%d)", got, len(got))
	}
	if got == long {
		t.Fatal("expected truncation for over-limit name")
	}
}
