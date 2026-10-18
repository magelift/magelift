package naming

import "testing"

func TestResourceTruncates(t *testing.T) {
	got := Resource("verylongprojectname", "verylongenvironmentname", "networksuffix")
	if len(got) > 63 {
		t.Fatalf("name too long: %q", got)
	}
}

func TestClusterName(t *testing.T) {
	if got := ClusterName("shop", "preview"); got != "shop-preview-mks" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatURL(t *testing.T) {
	if got := FormatURL("1.2.3.4"); got != "https://1.2.3.4" {
		t.Fatalf("got %q", got)
	}
}
