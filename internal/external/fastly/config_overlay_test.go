package fastly

import (
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestApplyAdvancedConfigPreservesPortableIntentAndMapsProviderFields(t *testing.T) {
	base := Config{TLS: true, TLSMode: "managed", OriginHealthRef: "health/shop", OriginPort: 443}
	got, err := ApplyAdvancedConfig(base, map[string]any{
		"version":           "7",
		"activateVersion":   true,
		"vclName":           "magelift-route",
		"vclContent":        "sub vcl_recv { return(pass); }",
		"originAddress":     "origin.internal.example",
		"originHost":        "shop.example.com",
		"originPort":        8443,
		"originUseTls":      true,
		"backendName":       "magelift_origin",
		"routeSnippetName":  "magelift-route-snippet",
		"tlsSubscriptionId": "tls-subscription-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.TLS || got.TLSMode != "managed" || got.OriginHealthRef != "health/shop" {
		t.Fatalf("portable intent was changed: %#v", got)
	}
	if got.Version != "7" || !got.ActivateVersion || got.VCLName != "magelift-route" || got.VCLContent == "" || got.OriginAddress != "origin.internal.example" || got.OriginHost != "shop.example.com" || got.OriginPort != 8443 || !got.OriginUseTLS || got.BackendName != "magelift_origin" || got.RouteSnippetName != "magelift-route-snippet" || got.TLSSubscriptionID != "tls-subscription-1" {
		t.Fatalf("advanced Fastly configuration = %#v", got)
	}
}

func TestApplyAdvancedConfigRejectsUnknownOrInvalidFields(t *testing.T) {
	for name, raw := range map[string]any{
		"unknown":      map[string]any{"originAddress": "origin.example", "secret": "must-not-be-accepted"},
		"invalid type": map[string]any{"originPort": "443"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplyAdvancedConfig(Config{}, raw); err == nil {
				t.Fatal("invalid advanced Fastly configuration was accepted")
			}
		})
	}
}

func TestConfigFromIntentReadsNamespacedAdvancedConfig(t *testing.T) {
	config, err := configFromIntent(sdk.EdgeIntent{ExternalProvider: "fastly", TLS: true, TLSMode: "managed"}, map[string]any{
		"extensions": map[string]any{
			"fastly.edge": map[string]any{"originAddress": "origin.example", "originPort": float64(443)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.OriginAddress != "origin.example" || config.OriginPort != 443 || !config.TLS {
		t.Fatalf("namespaced Fastly configuration = %#v", config)
	}
}

func TestConfigFromIntentRejectsMalformedNamespacedAdvancedConfig(t *testing.T) {
	_, err := configFromIntent(sdk.EdgeIntent{ExternalProvider: "fastly"}, map[string]any{
		"extensions": map[string]any{"fastly.edge": map[string]any{"token": "plaintext"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("malformed namespaced configuration error = %v", err)
	}
}
