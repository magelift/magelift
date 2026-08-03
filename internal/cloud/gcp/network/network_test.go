package network

import (
	"fmt"
	"net/netip"
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
}

type networkMocks struct {
	mu    sync.Mutex
	nodes []resourceNode
}

func (mocks *networkMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	mocks.mu.Lock()
	mocks.nodes = append(mocks.nodes, resourceNode{typeToken: args.TypeToken, name: args.Name})
	mocks.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}

func (*networkMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (mocks *networkMocks) snapshot() []string {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	result := make([]string, len(mocks.nodes))
	for index, node := range mocks.nodes {
		result[index] = node.typeToken + ":" + node.name
	}
	return result
}

func TestSubnetCIDRsBoundaries(t *testing.T) {
	t.Parallel()
	prefix := netip.MustParsePrefix("10.20.0.0/20")
	availableSlash24s := 1 << (24 - prefix.Bits())
	maxIndex := availableSlash24s/2 - 1

	t.Run("min", func(t *testing.T) {
		t.Parallel()
		privateCIDR, publicCIDR, err := subnetCIDRs(prefix, 0)
		if err != nil {
			t.Fatal(err)
		}
		if privateCIDR != "10.20.0.0/24" || publicCIDR != "10.20.1.0/24" {
			t.Fatalf("index 0 = %s,%s", privateCIDR, publicCIDR)
		}
		assertContained(t, prefix, privateCIDR, publicCIDR)
		private := netip.MustParsePrefix(privateCIDR)
		public := netip.MustParsePrefix(publicCIDR)
		if private.Addr().Compare(public.Addr()) >= 0 {
			t.Fatal("private range should precede public")
		}
	})

	t.Run("max", func(t *testing.T) {
		t.Parallel()
		privateCIDR, publicCIDR, err := subnetCIDRs(prefix, maxIndex)
		if err != nil {
			t.Fatal(err)
		}
		if privateCIDR != "10.20.14.0/24" || publicCIDR != "10.20.15.0/24" {
			t.Fatalf("index %d = %s,%s", maxIndex, privateCIDR, publicCIDR)
		}
		assertContained(t, prefix, privateCIDR, publicCIDR)
	})

	t.Run("max+1", func(t *testing.T) {
		t.Parallel()
		_, _, err := subnetCIDRs(prefix, maxIndex+1)
		if err == nil {
			t.Fatal("expected index overflow error")
		}
		if !strings.Contains(err.Error(), fmt.Sprint(availableSlash24s/2)) {
			t.Fatalf("error should name index limit: %v", err)
		}
	})
}

func TestSubnetCIDRsRejectsNarrowerThanSlash20(t *testing.T) {
	t.Parallel()
	_, _, err := subnetCIDRs(netip.MustParsePrefix("10.20.0.0/21"), 0)
	if err == nil {
		t.Fatal("expected /20-or-wider error")
	}
	if !strings.Contains(err.Error(), "at most /20") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRejectsOverlongZoneListBeforeRegistration(t *testing.T) {
	t.Parallel()
	prefix := netip.MustParsePrefix("10.20.0.0/20")
	maxZones := (1 << (24 - prefix.Bits())) / 2
	zones := make([]string, maxZones+1)
	for i := range zones {
		zones[i] = fmt.Sprintf("europe-west1-%c", 'b'+i)
	}
	mocks := &networkMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop", Args{
			Preset: sdk.PresetPreview, Project: "demo", Region: "europe-west1",
			NetworkCIDR: prefix.String(), Zones: zones,
		})
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
	if err == nil {
		t.Fatal("over-long zone list was accepted")
	}
	if !strings.Contains(err.Error(), fmt.Sprint(maxZones)) {
		t.Fatalf("error should name zone capacity: %v", err)
	}
	if len(mocks.snapshot()) != 0 {
		t.Fatalf("resources registered before zone capacity rejection: %v", mocks.snapshot())
	}
}

func assertContained(t *testing.T, parent netip.Prefix, cidrs ...string) {
	t.Helper()
	for _, cidr := range cidrs {
		block := netip.MustParsePrefix(cidr)
		if !parent.Contains(block.Addr()) || !parent.Contains(lastAddr(block)) {
			t.Fatalf("%s not contained in %s", cidr, parent)
		}
	}
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
