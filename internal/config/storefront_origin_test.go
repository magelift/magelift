package config

import "testing"

func TestValidateStorefrontOrigin(t *testing.T) {
	for _, value := range []string{"https://shop.example", "http://localhost:3000", "https://[::1]:8443", "https://shop.example/"} {
		if err := ValidateStorefrontOrigin(value); err != nil {
			t.Errorf("valid origin %q: %v", value, err)
		}
	}
	for _, value := range []string{"*", "https://*.example", "null", "https://example/path", "https://user@example", "https://example?x=1", "https://example,https://evil", " https://example", "https://example#fragment"} {
		if err := ValidateStorefrontOrigin(value); err == nil {
			t.Errorf("accepted invalid origin %q", value)
		}
	}
}
