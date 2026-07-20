package naming

import "testing"

func TestClusterName(t *testing.T) {
	if got := ClusterName("shop", "preview"); got != "shop-preview-kapsule" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatURL(t *testing.T) {
	if got := FormatURL("1.2.3.4"); got != "https://1.2.3.4" {
		t.Fatalf("got %q", got)
	}
}
