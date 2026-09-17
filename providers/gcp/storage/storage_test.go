package storage

import (
	"strings"
	"testing"
)

func TestMediaPrefixContract(t *testing.T) {
	t.Parallel()
	if MediaPrefix != "media/" {
		t.Fatalf("MediaPrefix = %q (PHP lifecycle writer contract)", MediaPrefix)
	}
}

func TestServiceAccountIDIsValid(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"shop-preview-app-media": "shop-preview-app-media",
		"UPPER_And.Dots-99":      "upper-and-dots-99",
		"9leading-digit":         "m-9leading-digit",
		"averylongprojectname-that-exceeds-the-thirty-character-service-account-limit": "averylongprojectname-that-exce",
	} {
		got := saID(name)
		if len(got) > 30 {
			t.Fatalf("saID(%q) = %q (too long)", name, got)
		}
		if got != want {
			t.Fatalf("saID(%q) = %q, want %q", name, got, want)
		}
		for _, r := range got {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				t.Fatalf("saID(%q) = %q (invalid rune %q)", name, got, r)
			}
		}
		if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Fatalf("saID(%q) = %q (edge hyphen)", name, got)
		}
	}
}
