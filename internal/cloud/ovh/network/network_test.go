package network

import (
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
