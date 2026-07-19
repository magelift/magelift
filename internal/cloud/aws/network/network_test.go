package network

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type resourceNode struct {
	typeToken string
	name      string
	inputs    resource.PropertyMap
}

type networkMocks struct {
	mu    sync.Mutex
	nodes []resourceNode
}

func (mocks *networkMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	mocks.mu.Lock()
	mocks.nodes = append(mocks.nodes, resourceNode{typeToken: args.TypeToken, name: args.Name, inputs: args.Inputs.Copy()})
	mocks.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}

func (*networkMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func TestPreviewResourceGraphSnapshot(t *testing.T) {
	t.Parallel()
	mocks := deploy(t, Args{
		Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.40.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		GatewayEndpoints: []GatewayEndpoint{{Service: GatewayEndpointS3, Rationale: ReduceNATCostAndExposure}},
	})
	want := []string{
		"aws:ec2/eip:Eip:shop-nat-eip-01", "aws:ec2/internetGateway:InternetGateway:shop-igw", "aws:ec2/natGateway:NatGateway:shop-nat-01",
		"aws:ec2/route:Route:shop-private-default-01", "aws:ec2/route:Route:shop-public-default-01",
		"aws:ec2/route:Route:shop-private-default-02", "aws:ec2/route:Route:shop-public-default-02",
		"aws:ec2/routeTable:RouteTable:shop-data-routes-01", "aws:ec2/routeTable:RouteTable:shop-private-routes-01", "aws:ec2/routeTable:RouteTable:shop-public-routes-01",
		"aws:ec2/routeTable:RouteTable:shop-data-routes-02", "aws:ec2/routeTable:RouteTable:shop-private-routes-02", "aws:ec2/routeTable:RouteTable:shop-public-routes-02",
		"aws:ec2/routeTableAssociation:RouteTableAssociation:shop-data-association-01", "aws:ec2/routeTableAssociation:RouteTableAssociation:shop-private-association-01", "aws:ec2/routeTableAssociation:RouteTableAssociation:shop-public-association-01",
		"aws:ec2/routeTableAssociation:RouteTableAssociation:shop-data-association-02", "aws:ec2/routeTableAssociation:RouteTableAssociation:shop-private-association-02", "aws:ec2/routeTableAssociation:RouteTableAssociation:shop-public-association-02",
		"aws:ec2/subnet:Subnet:shop-data-subnet-01", "aws:ec2/subnet:Subnet:shop-private-subnet-01", "aws:ec2/subnet:Subnet:shop-public-subnet-01",
		"aws:ec2/subnet:Subnet:shop-data-subnet-02", "aws:ec2/subnet:Subnet:shop-private-subnet-02", "aws:ec2/subnet:Subnet:shop-public-subnet-02",
		"aws:ec2/vpc:Vpc:shop-vpc", "aws:ec2/vpcEndpoint:VpcEndpoint:shop-endpoint-s3", TypeToken + ":shop",
	}
	sort.Strings(want)
	if got := mocks.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("resource graph:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := mocks.values("aws:ec2/subnet:Subnet", "cidrBlock"); !reflect.DeepEqual(got, []string{"10.40.0.0/20", "10.40.16.0/20", "10.40.32.0/20", "10.40.48.0/20", "10.40.64.0/20", "10.40.80.0/20"}) {
		t.Fatalf("subnet CIDRs = %v", got)
	}
	endpoint := mocks.one(t, "aws:ec2/vpcEndpoint:VpcEndpoint")
	if got := len(endpoint.inputs[resource.PropertyKey("routeTableIds")].ArrayValue()); got != 4 {
		t.Fatalf("S3 endpoint route table count = %d, want 4", got)
	}
}

func TestPresetNATAndZoneGraphs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset sdk.PresetID
		zones  []string
		nats   int
		nodes  int
	}{
		{sdk.PresetPreview, []string{"eu-west-3a", "eu-west-3b"}, 1, 27},
		{sdk.PresetStandard, []string{"eu-west-3a", "eu-west-3b"}, 2, 29},
		{sdk.PresetHighAvailability, []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}, 3, 42},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.preset), func(t *testing.T) {
			t.Parallel()
			mocks := deploy(t, Args{Preset: test.preset, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: test.zones})
			if got := mocks.count("aws:ec2/natGateway:NatGateway"); got != test.nats {
				t.Fatalf("NAT gateways = %d, want %d", got, test.nats)
			}
			if got := len(mocks.snapshot()); got != test.nodes {
				t.Fatalf("resource count = %d, want %d", got, test.nodes)
			}
			if got := mocks.count("aws:ec2/route:Route"); got != len(test.zones)*2 {
				t.Fatalf("default routes = %d", got)
			}
		})
	}
}

func TestStandardSupportsThreeZonesForRabbitMQDataPlane(t *testing.T) {
	t.Parallel()
	mocks := deploy(t, Args{Preset: sdk.PresetStandard, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}})
	if got := mocks.count("aws:ec2/natGateway:NatGateway"); got != 3 {
		t.Fatalf("NAT gateways = %d, want 3", got)
	}
}

func TestStandardAddsPrivateInterfaceEndpointsForRuntimeDependencies(t *testing.T) {
	t.Parallel()
	mocks := deploy(t, Args{
		Preset: sdk.PresetStandard, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		InterfaceEndpoints: []InterfaceEndpoint{
			{Service: InterfaceEndpointECRAPI, Rationale: ReduceNATCostAndExposure},
			{Service: InterfaceEndpointECRDKR, Rationale: ReduceNATCostAndExposure},
			{Service: InterfaceEndpointLogs, Rationale: ReduceNATCostAndExposure},
		},
	})
	if got := mocks.count("aws:ec2/vpcEndpoint:VpcEndpoint"); got != 3 {
		t.Fatalf("interface endpoint count = %d", got)
	}
	endpoint := mocks.one(t, "aws:ec2/vpcEndpoint:VpcEndpoint")
	if got := endpoint.inputs[resource.PropertyKey("vpcEndpointType")].StringValue(); got != "Interface" {
		t.Fatalf("endpoint type = %q", got)
	}
	if !endpoint.inputs[resource.PropertyKey("privateDnsEnabled")].BoolValue() {
		t.Fatal("private DNS is disabled")
	}
}

func TestExistingNetworkUsesImportedSubnetsWithoutCreatingVPCResources(t *testing.T) {
	t.Parallel()
	mocks := deploy(t, Args{
		Preset: sdk.PresetStandard, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		Existing: &ExistingNetwork{VPCID: "vpc-existing", PublicSubnetIDs: []string{"subnet-public-a", "subnet-public-b"}, PrivateSubnetIDs: []string{"subnet-private-a", "subnet-private-b"}, DataSubnetIDs: []string{"subnet-data-a", "subnet-data-b"}},
	})
	if got := mocks.count("aws:ec2/vpc:Vpc") + mocks.count("aws:ec2/subnet:Subnet") + mocks.count("aws:ec2/natGateway:NatGateway"); got != 0 {
		t.Fatalf("existing network created managed network resources: %v", mocks.snapshot())
	}
	if !contains(mocks.snapshot(), TypeToken+":shop") {
		t.Fatal("existing network component was not registered")
	}
}

func TestNetworkRejectsInvalidInputsBeforeRegistration(t *testing.T) {
	t.Parallel()
	tests := []Args{
		{Preset: sdk.PresetStandard, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a"}},
		{Preset: sdk.PresetPreview, Region: "", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}},
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.1.0/25", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}},
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, GatewayEndpoints: []GatewayEndpoint{{Service: GatewayEndpointS3}}},
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, InterfaceEndpoints: []InterfaceEndpoint{{Service: "unsupported", Rationale: ReduceNATCostAndExposure}}},
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, InterfaceEndpoints: []InterfaceEndpoint{{Service: InterfaceEndpointLogs, Rationale: ""}}},
	}
	for index, args := range tests {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			mocks := &networkMocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", mocks))
			if err == nil {
				t.Fatal("invalid network was accepted")
			}
			if len(mocks.snapshot()) != 0 {
				t.Fatal("resources registered before input validation")
			}
		})
	}
}

func deploy(t *testing.T, args Args) *networkMocks {
	t.Helper()
	mocks := &networkMocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", mocks)); err != nil {
		t.Fatal(err)
	}
	return mocks
}

func (mocks *networkMocks) snapshot() []string {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	result := make([]string, len(mocks.nodes))
	for index, node := range mocks.nodes {
		result[index] = node.typeToken + ":" + node.name
	}
	sort.Strings(result)
	return result
}

func (mocks *networkMocks) count(typeToken string) int {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	count := 0
	for _, node := range mocks.nodes {
		if node.typeToken == typeToken {
			count++
		}
	}
	return count
}

func (mocks *networkMocks) values(typeToken, key string) []string {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	values := []string{}
	for _, node := range mocks.nodes {
		if node.typeToken == typeToken {
			values = append(values, node.inputs[resource.PropertyKey(key)].StringValue())
		}
	}
	sort.Strings(values)
	return values
}

func (mocks *networkMocks) one(t *testing.T, typeToken string) resourceNode {
	t.Helper()
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	for _, node := range mocks.nodes {
		if node.typeToken == typeToken {
			return node
		}
	}
	t.Fatalf("resource type %s not found", typeToken)
	return resourceNode{}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
