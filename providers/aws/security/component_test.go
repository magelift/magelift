package security

import (
	"reflect"
	"sort"
	"sync"
	"testing"

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
	return args.Name + "-id", args.Inputs, nil
}

func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestCreatesStableLeastPrivilegeSecurityGraph(t *testing.T) {
	t.Parallel()
	m, outputs := deploy(t, Args{Region: "eu-west-3", VPCID: pulumi.String("vpc-123"), Tags: map[string]string{"magelift:managed-by": "magelift"}})

	if got := m.count("aws:ec2/securityGroup:SecurityGroup"); got != 6 {
		t.Fatalf("security groups = %d", got)
	}
	if got := m.count("aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule"); got != 6 {
		t.Fatalf("ingress rules = %d", got)
	}
	if got := m.count("aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule"); got != 7 {
		t.Fatalf("egress rules = %d", got)
	}

	wantGroups := []string{"shop-cache", "shop-data", "shop-edge", "shop-queue", "shop-search", "shop-web"}
	if got := m.names("aws:ec2/securityGroup:SecurityGroup"); !reflect.DeepEqual(got, wantGroups) {
		t.Fatalf("security group names = %v", got)
	}
	for _, group := range m.ofType("aws:ec2/securityGroup:SecurityGroup") {
		if group.inputs["vpcId"].StringValue() != "vpc-123" || group.inputs["region"].StringValue() != "eu-west-3" {
			t.Fatalf("group %s placement = %v", group.name, group.inputs)
		}
		if len(group.inputs["ingress"].ArrayValue()) != 0 || len(group.inputs["egress"].ArrayValue()) != 0 {
			t.Fatalf("group %s has inline rules", group.name)
		}
	}

	assertRule(t, m, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-ingress-edge-https", "shop-edge-id", "", "0.0.0.0/0", 443)
	assertRule(t, m, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-ingress-web-target", "shop-web-id", "shop-edge-id", "", 8080)
	assertRule(t, m, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-ingress-data-mysql", "shop-data-id", "shop-web-id", "", 3306)
	assertRule(t, m, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-ingress-cache-valkey", "shop-cache-id", "shop-web-id", "", 6379)
	assertRule(t, m, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-ingress-search-https", "shop-search-id", "shop-web-id", "", 443)
	assertRule(t, m, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-ingress-queue-amqps", "shop-queue-id", "shop-web-id", "", 5671)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-edge-target", "shop-edge-id", "shop-web-id", "", 8080)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-web-data", "shop-web-id", "shop-data-id", "", 3306)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-web-cache", "shop-web-id", "shop-cache-id", "", 6379)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-web-search", "shop-web-id", "shop-search-id", "", 443)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-web-queue", "shop-web-id", "shop-queue-id", "", 5671)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-web-https", "shop-web-id", "", "0.0.0.0/0", 443)
	assertRule(t, m, "aws:vpc/securityGroupEgressRule:SecurityGroupEgressRule", "shop-egress-queue-https", "shop-queue-id", "", "0.0.0.0/0", 443)

	for _, ingress := range m.ofType("aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule") {
		cidr, hasCIDR := ingress.inputs["cidrIpv4"]
		if hasCIDR && cidr.StringValue() == "0.0.0.0/0" && ingress.name != "shop-ingress-edge-https" {
			t.Fatalf("broad inbound CIDR on %s", ingress.name)
		}
	}

	wantOutputs := map[string]string{
		"edge": "shop-edge-id", "web": "shop-web-id", "data": "shop-data-id", "cache": "shop-cache-id",
		"search": "shop-search-id", "queue": "shop-queue-id",
	}
	if !reflect.DeepEqual(outputs, wantOutputs) {
		t.Fatalf("outputs = %v", outputs)
	}
}

func TestRejectsMissingPlacementBeforeRegistration(t *testing.T) {
	t.Parallel()
	tests := []Args{
		{VPCID: pulumi.String("vpc-123")},
		{Region: "eu-west-3"},
	}
	for index, args := range tests {
		m := &mocks{}
		err := pulumi.RunErr(func(ctx *pulumi.Context) error {
			_, err := New(ctx, "shop", args)
			return err
		}, pulumi.WithMocks("project", "stack", m))
		if err == nil {
			t.Fatalf("case %d was accepted", index)
		}
		if got := m.snapshot(); len(got) != 0 {
			t.Fatalf("case %d registered resources: %v", index, got)
		}
	}
}

func deploy(t *testing.T, args Args) (*mocks, map[string]string) {
	t.Helper()
	m := &mocks{}
	outputs := map[string]string{}
	var outputsMu sync.Mutex
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		component, err := New(ctx, "shop", args)
		if err != nil {
			return err
		}
		values := []struct {
			name  string
			value pulumi.StringOutput
		}{
			{"edge", component.EdgeSecurityGroupID}, {"web", component.WebSecurityGroupID}, {"data", component.DataSecurityGroupID},
			{"cache", component.CacheSecurityGroupID}, {"search", component.SearchSecurityGroupID}, {"queue", component.QueueSecurityGroupID},
		}
		for _, value := range values {
			value := value
			value.value.ApplyT(func(resolved string) string {
				outputsMu.Lock()
				outputs[value.name] = resolved
				outputsMu.Unlock()
				return resolved
			})
		}
		return nil
	}, pulumi.WithMocks("project", "stack", m)); err != nil {
		t.Fatal(err)
	}
	return m, outputs
}

func assertRule(t *testing.T, m *mocks, token, name, groupID, referencedID, cidr string, port float64) {
	t.Helper()
	rule := m.named(t, token, name).inputs
	if rule["securityGroupId"].StringValue() != groupID || rule["ipProtocol"].StringValue() != "tcp" || rule["fromPort"].NumberValue() != port || rule["toPort"].NumberValue() != port {
		t.Fatalf("rule %s = %v", name, rule)
	}
	if referencedID != "" && rule["referencedSecurityGroupId"].StringValue() != referencedID {
		t.Fatalf("rule %s source group = %v", name, rule)
	}
	if cidr != "" && rule["cidrIpv4"].StringValue() != cidr {
		t.Fatalf("rule %s CIDR = %v", name, rule)
	}
}

func (m *mocks) ofType(token string) []node {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []node{}
	for _, node := range m.nodes {
		if node.typeToken == token {
			result = append(result, node)
		}
	}
	return result
}

func (m *mocks) count(token string) int { return len(m.ofType(token)) }

func (m *mocks) names(token string) []string {
	nodes := m.ofType(token)
	result := make([]string, len(nodes))
	for index, node := range nodes {
		result[index] = node.name
	}
	sort.Strings(result)
	return result
}

func (m *mocks) named(t *testing.T, token, name string) node {
	t.Helper()
	for _, node := range m.ofType(token) {
		if node.name == name {
			return node
		}
	}
	t.Fatalf("resource %s %s not found", token, name)
	return node{}
}

func (m *mocks) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.nodes))
	for index, node := range m.nodes {
		result[index] = node.typeToken + ":" + node.name
	}
	sort.Strings(result)
	return result
}
