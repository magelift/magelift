package network

import (
	"fmt"
	"net/netip"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/sdk"
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
	if got := subnetTag(mocks.named(t, "aws:ec2/subnet:Subnet", "shop-public-subnet-01"), "kubernetes.io/role/elb"); got != "1" {
		t.Fatalf("public subnet kubernetes.io/role/elb = %q, want 1", got)
	}
	if got := subnetTag(mocks.named(t, "aws:ec2/subnet:Subnet", "shop-private-subnet-01"), "kubernetes.io/role/internal-elb"); got != "1" {
		t.Fatalf("private subnet kubernetes.io/role/internal-elb = %q, want 1", got)
	}
	if got := subnetTag(mocks.named(t, "aws:ec2/subnet:Subnet", "shop-data-subnet-01"), "kubernetes.io/role/elb"); got != "" {
		t.Fatalf("data subnet must not advertise ELB discovery, got %q", got)
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

func TestFckNatTopologyControlsNATInstanceCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		topology   string
		wantNATs   int
		wantRoutes int
	}{
		{name: "single-az", topology: NatTopologySingleAZ, wantNATs: 1, wantRoutes: 2},
		{name: "multi-az", topology: NatTopologyMultiAZ, wantNATs: 2, wantRoutes: 2},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mocks := deploy(t, Args{
				Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.40.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
				NatMode: NatModeFckNat, NatTopology: test.topology, NatReplacementMode: NatReplacementNone,
			})
			if got := mocks.count("aws:ec2/instance:Instance"); got != test.wantNATs {
				t.Fatalf("fck-nat instances = %d, want %d", got, test.wantNATs)
			}
			if got := mocks.count("aws:ec2/natGateway:NatGateway"); got != 0 {
				t.Fatalf("fck-nat created managed NAT gateways = %d", got)
			}
			privateRoutes := 0
			for _, node := range mocks.nodes {
				if node.typeToken == "aws:ec2/route:Route" && strings.Contains(node.name, "private-default") {
					privateRoutes++
				}
			}
			if privateRoutes != test.wantRoutes {
				t.Fatalf("private default routes = %d, want %d", privateRoutes, test.wantRoutes)
			}
		})
	}
}

func TestFckNatAutoScalingUsesStableNetworkInterfaces(t *testing.T) {
	t.Parallel()
	mocks := deploy(t, Args{
		Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.40.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		NatMode: NatModeFckNat, NatTopology: NatTopologyMultiAZ, NatReplacementMode: NatReplacementAutoScaling,
	})
	if got := mocks.count("aws:ec2/instance:Instance"); got != 0 {
		t.Fatalf("auto-scaling fck-nat created unmanaged instances = %d", got)
	}
	if got := mocks.count("aws:autoscaling/group:Group"); got != 2 {
		t.Fatalf("fck-nat auto-scaling groups = %d, want 2", got)
	}
	if got := mocks.count("aws:ec2/networkInterface:NetworkInterface"); got != 2 {
		t.Fatalf("stable fck-nat interfaces = %d, want 2", got)
	}
	if got := mocks.count("aws:ec2/launchTemplate:LaunchTemplate"); got != 2 {
		t.Fatalf("fck-nat launch templates = %d, want 2", got)
	}
	for index := 1; index <= 2; index++ {
		var route resourceNode
		for _, node := range mocks.nodes {
			if node.typeToken == "aws:ec2/route:Route" && node.name == fmt.Sprintf("shop-private-default-%02d", index) {
				route = node
				break
			}
		}
		if value := route.inputs[resource.PropertyKey("networkInterfaceId")].StringValue(); !strings.Contains(value, "fck-nat-eni") {
			t.Fatalf("private route %d target = %q, want stable fck-nat ENI", index, value)
		}
	}
}

func TestFckNatUsesDocumentedAMIAndCostOptimizedDefaultInstance(t *testing.T) {
	if FckNatAMIOwner != "568608671756" || FckNatAMINamePrefix != "fck-nat-al2023-*-arm64-ebs" {
		t.Fatalf("fck-nat AMI contract changed: owner=%q name=%q", FckNatAMIOwner, FckNatAMINamePrefix)
	}
	if got := resolveNatInstanceType("", NatModeFckNat); got != "t4g.nano" {
		t.Fatalf("default fck-nat instance type = %q, want t4g.nano", got)
	}
	if got := resolveNatInstanceType("c6gn.medium", NatModeFckNat); got != "c6gn.medium" {
		t.Fatalf("custom fck-nat instance type = %q", got)
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
	const vpcID = "vpc-existing"
	mocks := deploy(t, Args{
		Preset: sdk.PresetStandard, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		Existing: &ExistingNetwork{VPCID: vpcID, PublicSubnetIDs: []string{"subnet-public-a", "subnet-public-b"}, PrivateSubnetIDs: []string{"subnet-private-a", "subnet-private-b"}, DataSubnetIDs: []string{"subnet-data-a", "subnet-data-b"}},
	})
	// ATTACH-01 / D-01: existing network is reference-without-own; zero managed VPC/subnet/NAT children.
	// Preview ADOPT lines for this externalId are covered by stack.AdoptReport (08-01).
	if got := mocks.count("aws:ec2/vpc:Vpc") + mocks.count("aws:ec2/subnet:Subnet") + mocks.count("aws:ec2/natGateway:NatGateway"); got != 0 {
		t.Fatalf("existing network created managed network resources: %v", mocks.snapshot())
	}
	if !strings.HasPrefix(vpcID, "vpc-") {
		t.Fatalf("existing VPC id %q is not AdoptReport-compatible", vpcID)
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
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat, NatTopology: "zone-local"},
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeGateway, NatReplacementMode: NatReplacementAutoScaling},
		{Preset: sdk.PresetPreview, Region: "eu-west-3", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat, NatReplacementMode: "recreate"},
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

func TestSubnetCarveBoundaries(t *testing.T) {
	t.Parallel()
	prefix := netip.MustParsePrefix("10.40.0.0/16")
	// Carve widens by 4 bits → 16 slots; each zone consumes 3 (public/private/data).
	const slotsPerZone = 3
	available := 1 << uint((prefix.Bits()+4)-prefix.Bits())
	maxZones := available / slotsPerZone

	t.Run("min", func(t *testing.T) {
		t.Parallel()
		zones := 1
		cidrs := subnetCIDRs(prefix, zones*slotsPerZone)
		if len(cidrs) != zones*slotsPerZone {
			t.Fatalf("cidr count = %d, want %d", len(cidrs), zones*slotsPerZone)
		}
		seen := map[string]bool{}
		for _, cidr := range cidrs {
			if seen[cidr] {
				t.Fatalf("duplicate CIDR %s", cidr)
			}
			seen[cidr] = true
			block := netip.MustParsePrefix(cidr)
			if !prefix.Contains(block.Addr()) || !prefix.Contains(lastAddr(block)) {
				t.Fatalf("%s not contained in %s", cidr, prefix)
			}
		}
		if err := validateCarveCapacity(prefix, zones); err != nil {
			t.Fatalf("min zones should fit: %v", err)
		}
	})

	t.Run("max", func(t *testing.T) {
		t.Parallel()
		cidrs := subnetCIDRs(prefix, maxZones*slotsPerZone)
		if len(cidrs) != maxZones*slotsPerZone {
			t.Fatalf("cidr count = %d, want %d", len(cidrs), maxZones*slotsPerZone)
		}
		last := netip.MustParsePrefix(cidrs[len(cidrs)-1])
		if !prefix.Contains(last.Addr()) || !prefix.Contains(lastAddr(last)) {
			t.Fatalf("last carved block %s not contained in %s", last, prefix)
		}
		if err := validateCarveCapacity(prefix, maxZones); err != nil {
			t.Fatalf("max zones should fit: %v", err)
		}
	})

	t.Run("max+1", func(t *testing.T) {
		t.Parallel()
		err := validateCarveCapacity(prefix, maxZones+1)
		if err == nil {
			t.Fatal("expected carve capacity error")
		}
		demand := (maxZones + 1) * slotsPerZone
		if !strings.Contains(err.Error(), fmt.Sprint(available)) || !strings.Contains(err.Error(), fmt.Sprint(demand)) {
			t.Fatalf("error should name available=%d and demanded=%d: %v", available, demand, err)
		}

		zones := zonesNamed(maxZones + 1)
		mocks := &networkMocks{}
		runErr := pulumi.RunErr(func(ctx *pulumi.Context) error {
			_, err := New(ctx, "shop", Args{
				Preset: sdk.PresetHighAvailability, Region: "eu-west-3", VPCCIDR: prefix.String(),
				AvailabilityZones: zones,
			})
			return err
		}, pulumi.WithMocks("project", "stack", mocks))
		if runErr == nil {
			t.Fatal("overflow zone count was accepted")
		}
		if !strings.Contains(runErr.Error(), fmt.Sprint(available)) || !strings.Contains(runErr.Error(), fmt.Sprint(demand)) {
			t.Fatalf("New error should name carve limit: %v", runErr)
		}
		if len(mocks.snapshot()) != 0 {
			t.Fatalf("resources registered before carve rejection: %v", mocks.snapshot())
		}
	})
}

func zonesNamed(n int) []string {
	zones := make([]string, n)
	for i := range zones {
		zones[i] = fmt.Sprintf("eu-west-3%c", 'a'+i)
	}
	return zones
}

func lastAddr(prefix netip.Prefix) netip.Addr {
	addr := prefix.Addr().As4()
	base := uint32(addr[0])<<24 | uint32(addr[1])<<16 | uint32(addr[2])<<8 | uint32(addr[3])
	mask := uint32(0xffffffff) >> uint(prefix.Bits())
	end := base | mask
	var raw [4]byte
	raw[0] = byte(end >> 24)
	raw[1] = byte(end >> 16)
	raw[2] = byte(end >> 8)
	raw[3] = byte(end)
	return netip.AddrFrom4(raw)
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

func (mocks *networkMocks) named(t *testing.T, typeToken, name string) resourceNode {
	t.Helper()
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	for _, node := range mocks.nodes {
		if node.typeToken == typeToken && node.name == name {
			return node
		}
	}
	t.Fatalf("resource %s:%s not found", typeToken, name)
	return resourceNode{}
}

func subnetTag(node resourceNode, key string) string {
	tags := node.inputs[resource.PropertyKey("tags")]
	if !tags.IsObject() {
		return ""
	}
	value := tags.ObjectValue()[resource.PropertyKey(key)]
	if !value.IsString() {
		return ""
	}
	return value.StringValue()
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
