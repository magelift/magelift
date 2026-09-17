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

func TestMediaObjectKeyFollowsMagentoURIMapping(t *testing.T) {
	t.Parallel()
	// Pinned to magento/magento2 2.4.9: the remote_storage root prefix
	// stays empty and Magento appends the MEDIA directory URI ("media")
	// below it, so media-relative paths map under media/ while
	// VAR_IMPORT_EXPORT lands at the sibling import_export/ subtree.
	if MediaPrefix != "media/" {
		t.Fatalf("MediaPrefix = %q, want the Magento MEDIA URI", MediaPrefix)
	}
	if got := MediaObjectKey("catalog/product/a.jpg"); got != "media/catalog/product/a.jpg" {
		t.Fatalf("MediaObjectKey = %q", got)
	}
}
