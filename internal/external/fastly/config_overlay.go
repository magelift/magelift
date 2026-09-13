package fastly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const fastlyEdgeExtensionKey = "fastly.edge"

// configOverlay is the provider-owned advanced YAML surface. Common edge
// intent stays in sdk.EdgeIntent; these fields are only the Fastly details
// that cannot be represented portably. Pointers distinguish an omitted value
// from an explicit zero or false value.
type configOverlay struct {
	Version           *string `json:"version"`
	ActivateVersion   *bool   `json:"activateVersion"`
	TLSSubscriptionID *string `json:"tlsSubscriptionId"`
	VCLName           *string `json:"vclName"`
	VCLContent        *string `json:"vclContent"`
	OriginAddress     *string `json:"originAddress"`
	OriginHost        *string `json:"originHost"`
	OriginPort        *int    `json:"originPort"`
	OriginUseTLS      *bool   `json:"originUseTls"`
	BackendName       *string `json:"backendName"`
	RouteSnippetName  *string `json:"routeSnippetName"`
}

// applyConfigOverlay decodes one namespaced extension payload without
// importing internal/config or exposing Fastly SDK models to the core. A
// strict decoder is deliberate: silently ignored provider settings create a
// dangerous configuration-success/behavior-mismatch at the edge.
func applyConfigOverlay(base Config, raw any) (Config, error) {
	if raw == nil {
		return base, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return Config{}, fmt.Errorf("encode Fastly advanced configuration: %w", err)
	}
	var overlay configOverlay
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&overlay); err != nil {
		return Config{}, fmt.Errorf("decode Fastly advanced configuration: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode Fastly advanced configuration: trailing data")
		}
		return Config{}, fmt.Errorf("decode Fastly advanced configuration: %w", err)
	}
	if overlay.Version != nil {
		base.Version = *overlay.Version
	}
	if overlay.ActivateVersion != nil {
		base.ActivateVersion = *overlay.ActivateVersion
	}
	if overlay.TLSSubscriptionID != nil {
		base.TLSSubscriptionID = *overlay.TLSSubscriptionID
	}
	if overlay.VCLName != nil {
		base.VCLName = *overlay.VCLName
	}
	if overlay.VCLContent != nil {
		base.VCLContent = *overlay.VCLContent
	}
	if overlay.OriginAddress != nil {
		base.OriginAddress = *overlay.OriginAddress
	}
	if overlay.OriginHost != nil {
		base.OriginHost = *overlay.OriginHost
	}
	if overlay.OriginPort != nil {
		base.OriginPort = *overlay.OriginPort
	}
	if overlay.OriginUseTLS != nil {
		base.OriginUseTLS = *overlay.OriginUseTLS
	}
	if overlay.BackendName != nil {
		base.BackendName = *overlay.BackendName
	}
	if overlay.RouteSnippetName != nil {
		base.RouteSnippetName = *overlay.RouteSnippetName
	}
	return base, nil
}

// ApplyAdvancedConfig is the CLI/provider construction boundary for the
// namespaced Fastly extension payload. It intentionally accepts only the
// provider-owned fields declared by configOverlay.
func ApplyAdvancedConfig(base Config, raw any) (Config, error) {
	return applyConfigOverlay(base, raw)
}

func fastlyExtensionConfig(configuration map[string]any) (any, bool) {
	extensions, ok := configuration["extensions"].(map[string]any)
	if !ok {
		return nil, false
	}
	value, ok := extensions[fastlyEdgeExtensionKey]
	return value, ok
}
