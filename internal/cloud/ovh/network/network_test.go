package network

import (
	"net/netip"
	"strings"
	"testing"
)

func TestValidateSubnetCarveOverflow(t *testing.T) {
	t.Parallel()
	_, err := validateSubnetCarve("10.30.0.0/24", []string{"GRA9", "GRA11"})
	if err == nil {
		t.Fatal("expected overflow error")
	}
	if !strings.Contains(err.Error(), "/24 subnet") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSubnetCarveNarrowerThanSlash24(t *testing.T) {
	t.Parallel()
	_, err := validateSubnetCarve("10.30.0.0/25", []string{"GRA9"})
	if err == nil {
		t.Fatal("expected /24-or-wider error")
	}
	if !strings.Contains(err.Error(), "/24 or wider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSubnetCarveOK(t *testing.T) {
	t.Parallel()
	prefix, err := validateSubnetCarve("10.30.0.0/23", []string{"GRA9", "GRA11"})
	if err != nil {
		t.Fatal(err)
	}
	if prefix.String() != "10.30.0.0/23" {
		t.Fatalf("got %s", prefix)
	}
}

func TestValidateSubnetCarveCapsWidePrefixes(t *testing.T) {
	t.Parallel()
	zones := make([]string, 17)
	for i := range zones {
		zones[i] = "Z"
	}
	_, err := validateSubnetCarve("10.0.0.0/16", zones)
	if err == nil {
		t.Fatal("expected cap at 16 /24 slots")
	}
	if !strings.Contains(err.Error(), "room for 16") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubnetCIDRIndexBoundaries(t *testing.T) {
	t.Parallel()
	parent := netip.MustParsePrefix("10.0.0.0/16")

	t.Run("index0", func(t *testing.T) {
		t.Parallel()
		cidr, err := subnetCIDR(parent, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cidr != "10.0.0.0/24" {
			t.Fatalf("index 0 = %s, want parent's own /24", cidr)
		}
		assertContained(t, parent, cidr)
	})

	t.Run("index15", func(t *testing.T) {
		t.Parallel()
		cidr, err := subnetCIDR(parent, 15)
		if err != nil {
			t.Fatal(err)
		}
		if cidr != "10.0.15.0/24" {
			t.Fatalf("index 15 = %s, want 10.0.15.0/24", cidr)
		}
		assertContained(t, parent, cidr)
	})

	t.Run("index16", func(t *testing.T) {
		t.Parallel()
		_, err := subnetCIDR(parent, 16)
		if err == nil {
			t.Fatal("expected index 16 out of range")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("negative", func(t *testing.T) {
		t.Parallel()
		_, err := subnetCIDR(parent, -1)
		if err == nil {
			t.Fatal("expected negative index rejection")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSubnetCIDRRejectsNarrowerThanSlash24(t *testing.T) {
	t.Parallel()
	_, err := subnetCIDR(netip.MustParsePrefix("10.30.0.0/25"), 0)
	if err == nil {
		t.Fatal("expected carve-time /24-or-wider error")
	}
	if !strings.Contains(err.Error(), "/24 or wider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSubnetCarveAcceptsSixteenZonesOnSlash16(t *testing.T) {
	t.Parallel()
	zones := make([]string, 16)
	for i := range zones {
		zones[i] = "Z"
	}
	prefix, err := validateSubnetCarve("10.0.0.0/16", zones)
	if err != nil {
		t.Fatalf("sixteen zones on /16 must be accepted: %v", err)
	}
	if prefix.String() != "10.0.0.0/16" {
		t.Fatalf("got %s", prefix)
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
