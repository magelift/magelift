package network

import (
	"sort"
	"strings"
	"sync"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type networkMocks struct {
	mu    sync.Mutex
	nodes []pulumi.MockResourceArgs
}

func (mocks *networkMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	mocks.mu.Lock()
	mocks.nodes = append(mocks.nodes, args)
	mocks.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "scaleway:network/vpc:Vpc":
		state["name"] = resource.NewStringProperty(args.Name)
	case "scaleway:network/privateNetwork:PrivateNetwork":
		state["name"] = resource.NewStringProperty(args.Name)
	}
	return args.Name + "-id", state, nil
}

func (*networkMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (mocks *networkMocks) snapshot() []string {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	result := make([]string, len(mocks.nodes))
	for index, node := range mocks.nodes {
		result[index] = node.TypeToken + ":" + node.Name
	}
	sort.Strings(result)
	return result
}

func (mocks *networkMocks) one(t *testing.T, typeToken string) pulumi.MockResourceArgs {
	t.Helper()
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	for _, node := range mocks.nodes {
		if node.TypeToken == typeToken {
			return node
		}
	}
	t.Fatalf("resource type %s not found", typeToken)
	return pulumi.MockResourceArgs{}
}

func (mocks *networkMocks) count(typeToken string) int {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	n := 0
	for _, node := range mocks.nodes {
		if node.TypeToken == typeToken {
			n++
		}
	}
	return n
}

func TestNewCreatesVpcAndPrivateNetworkWithSubnet(t *testing.T) {
	t.Parallel()
	mocks := &networkMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-preview-net", Args{
			Preset: sdk.PresetPreview, ProjectID: "11111111-1111-1111-1111-111111111111",
			Region: "fr-par", NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1"},
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		})
		return err
	}, pulumi.WithMocks("magelift", "shop-preview", mocks))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"magelift:scaleway:Network:shop-preview-net",
		"scaleway:network/privateNetwork:PrivateNetwork:shop-preview-net-pn",
		"scaleway:network/vpc:Vpc:shop-preview-net-vpc",
	}
	if got := mocks.snapshot(); !equalStrings(got, want) {
		t.Fatalf("resource graph = %v, want %v", got, want)
	}
	pn := mocks.one(t, "scaleway:network/privateNetwork:PrivateNetwork")
	subnet := pn.Inputs["ipv4Subnet"].ObjectValue()["subnet"].StringValue()
	if subnet != "172.16.0.0/22" {
		t.Fatalf("private network subnet = %q, want 172.16.0.0/22", subnet)
	}
	vpc := mocks.one(t, "scaleway:network/vpc:Vpc")
	if got := vpc.Inputs["projectId"].StringValue(); got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("vpc projectId = %q", got)
	}
}

func TestNewRejectsInvalidInputsBeforeRegistration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args Args
	}{
		{name: "missing zones", args: Args{ProjectID: "p", Region: "fr-par", NetworkCIDR: "172.16.0.0/22", Zones: nil}},
		{name: "missing project id", args: Args{ProjectID: "", Region: "fr-par", NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1"}}},
		{name: "missing region", args: Args{ProjectID: "p", Region: "", NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1"}}},
		{name: "malformed cidr", args: Args{ProjectID: "p", Region: "fr-par", NetworkCIDR: "not-a-cidr", Zones: []string{"fr-par-1"}}},
		{name: "ipv6 cidr", args: Args{ProjectID: "p", Region: "fr-par", NetworkCIDR: "2001:db8::/32", Zones: []string{"fr-par-1"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mocks := &networkMocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				_, err := New(ctx, "shop-net", tc.args)
				return err
			}, pulumi.WithMocks("magelift", "shop-preview", mocks))
			if err == nil {
				t.Fatalf("expected rejection for %+v", tc.args)
			}
			if len(mocks.snapshot()) != 0 {
				t.Fatalf("resources registered before input validation for %+v", tc.args)
			}
		})
	}
}

// TestNewKeepsSinglePrivateNetworkRange documents that Scaleway deliberately
// does not carve per-zone subnets. A Scaleway private network is the Magento
// attachment unit, so the configured NetworkCIDR is attached verbatim and
// PrivateSubnetIDs always holds exactly one entry — there is no index cap or
// "cap plus one" boundary to assert the way OVH/AWS/GCP carve helpers do.
func TestNewKeepsSinglePrivateNetworkRange(t *testing.T) {
	t.Parallel()
	const networkCIDR = "172.16.0.0/22"
	cases := []struct {
		name  string
		zones []string
	}{
		{name: "one zone", zones: []string{"fr-par-1"}},
		{name: "three zones", zones: []string{"fr-par-1", "fr-par-2", "fr-par-3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mocks := &networkMocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				_, err := New(ctx, "shop-net", Args{
					Preset: sdk.PresetPreview, ProjectID: "11111111-1111-1111-1111-111111111111",
					Region: "fr-par", NetworkCIDR: networkCIDR, Zones: tc.zones,
					Labels: map[string]string{"magelift-managed-by": "magelift"},
				})
				return err
			}, pulumi.WithMocks("magelift", "shop-preview", mocks))
			if err != nil {
				t.Fatal(err)
			}
			// One PN resource ⇒ PrivateSubnetIDs is the single-element list of that PN.
			if got := mocks.count("scaleway:network/privateNetwork:PrivateNetwork"); got != 1 {
				t.Fatalf("private network count = %d, want 1 regardless of %d zones", got, len(tc.zones))
			}
			pn := mocks.one(t, "scaleway:network/privateNetwork:PrivateNetwork")
			subnet := pn.Inputs["ipv4Subnet"].ObjectValue()["subnet"].StringValue()
			if subnet != networkCIDR {
				t.Fatalf("private network subnet = %q, want configured %q (no carve)", subnet, networkCIDR)
			}
			if pn.RegisterRPC == nil {
				t.Fatal("RegisterRPC missing; cannot assert VPC dependency")
			}
			foundVPC := false
			for _, dep := range pn.RegisterRPC.GetDependencies() {
				if strings.Contains(dep, "-vpc") {
					foundVPC = true
					break
				}
			}
			if !foundVPC {
				t.Fatalf("private network missing VPC dependency; deps=%v", pn.RegisterRPC.GetDependencies())
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
