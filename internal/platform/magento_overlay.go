package platform

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
)

const (
	EnvMagentoFrontName        = "MAGENTO_DC_BACKEND__FRONTNAME"
	EnvMagentoCookieDomain     = "CONFIG__DEFAULT__WEB__COOKIE__COOKIE_DOMAIN"
	EnvMagentoUnsecureBaseURL  = "CONFIG__DEFAULT__WEB__UNSECURE__BASE_URL"
	EnvMagentoSecureBaseURL    = "CONFIG__DEFAULT__WEB__SECURE__BASE_URL"
	EnvMagentoCORSOrigins      = "CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ALLOWED_ORIGINS"
	EnvMagentoCronConsumersRun = "MAGENTO_DC_CRON_CONSUMERS_RUNNER__CRON_RUN"
	EnvMagentoConsumerNameList = "MAGELIFT_MAGENTO_CONSUMERS"
)

// MagentoConsumerProcessCount is the dedicated Magento consumer replica count.
func MagentoConsumerProcessCount(mode string, catalogCount int) int {
	switch strings.TrimSpace(mode) {
	case "cron":
		return 0
	case "processes", "both":
		if catalogCount > 0 {
			return catalogCount
		}
		return 1
	default:
		return catalogCount
	}
}

// NewMagentoOverlays copies YAML Magento runtime fields onto the portable env contract.
func NewMagentoOverlays(frontName, cookieDomain, unsecureBaseURL, secureBaseURL, storefrontOrigin, consumersMode string, corsOrigins, consumerNames []string, variables map[string]string) MagentoOverlays {
	copied := make(map[string]string, len(variables))
	for key, value := range variables {
		copied[key] = value
	}
	return MagentoOverlays{
		FrontName:        strings.TrimSpace(frontName),
		CookieDomain:     strings.TrimSpace(cookieDomain),
		UnsecureBaseURL:  strings.TrimSpace(unsecureBaseURL),
		SecureBaseURL:    strings.TrimSpace(secureBaseURL),
		CORSOrigins:      append([]string(nil), corsOrigins...),
		StorefrontOrigin: strings.TrimSpace(storefrontOrigin),
		ConsumersMode:    strings.TrimSpace(consumersMode),
		ConsumerNames:    append([]string(nil), consumerNames...),
		Variables:        copied,
	}
}

// MagentoOverlays are portable Magento runtime values from magelift.yaml.
// Adapters inject them through MAGENTO_DC_* / CONFIG__* and MAGENTO_DC__OVERRIDE.
// MagentoOverlays is the SDK-governed Magento runtime contract. The struct
// moved to the SDK so deploy inputs are versioned; behavior stays here.
type MagentoOverlays = sdk.MagentoOverlays

// MagentoOverlayEnv returns scalar Magento runtime contract bindings.
func MagentoOverlayEnv(overlays MagentoOverlays) []EnvBinding {
	var bindings []EnvBinding
	if frontName := strings.TrimSpace(overlays.FrontName); frontName != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoFrontName, Value: frontName})
	}
	if cookie := strings.TrimSpace(overlays.CookieDomain); cookie != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoCookieDomain, Value: cookie})
	}
	if url := strings.TrimSpace(overlays.UnsecureBaseURL); url != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoUnsecureBaseURL, Value: url})
	}
	if url := strings.TrimSpace(overlays.SecureBaseURL); url != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoSecureBaseURL, Value: url})
	}
	if origins := magentoCORSOriginList(overlays); origins != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoCORSOrigins, Value: origins})
	}
	if cronRun, ok := magentoCronConsumersRun(overlays.ConsumersMode); ok {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoCronConsumersRun, Value: cronRun})
	}
	if names := strings.Join(nonEmptyStrings(overlays.ConsumerNames), ","); names != "" {
		bindings = append(bindings, EnvBinding{Name: EnvMagentoConsumerNameList, Value: names})
	}
	keys := make([]string, 0, len(overlays.Variables))
	for key := range overlays.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := overlays.Variables[key]
		if key == EnvMagentoDCOverride || strings.Contains(value, "://") {
			continue
		}
		bindings = append(bindings, EnvBinding{Name: key, Value: value})
	}
	return bindings
}

// MagentoOverlayJSON is the MAGENTO_DC__OVERRIDE fragment for arrays and env.php maps.
func MagentoOverlayJSON(overlays MagentoOverlays) string {
	overlay := map[string]any{}
	if frontName := strings.TrimSpace(overlays.FrontName); frontName != "" {
		overlay["backend"] = map[string]any{"frontName": frontName}
	}
	consumers := map[string]any{}
	if cronRun, ok := magentoCronConsumersRun(overlays.ConsumersMode); ok {
		consumers["cron_run"] = cronRun == "1"
	}
	if names := nonEmptyStrings(overlays.ConsumerNames); len(names) > 0 {
		consumers["consumers"] = names
	}
	if len(consumers) > 0 {
		overlay["cron_consumers_runner"] = consumers
	}
	web := map[string]any{}
	if cookie := strings.TrimSpace(overlays.CookieDomain); cookie != "" {
		web["cookie"] = map[string]any{"cookie_domain": cookie}
	}
	if url := strings.TrimSpace(overlays.UnsecureBaseURL); url != "" {
		web["unsecure"] = map[string]any{"base_url": url}
	}
	if url := strings.TrimSpace(overlays.SecureBaseURL); url != "" {
		web["secure"] = map[string]any{"base_url": url}
	}
	if origins := magentoCORSOriginList(overlays); origins != "" {
		web["graphql"] = map[string]any{"cors_allowed_origins": origins}
	}
	if len(web) > 0 {
		overlay["system"] = map[string]any{"default": map[string]any{"web": web}}
	}
	if userOverride := strings.TrimSpace(overlays.Variables[EnvMagentoDCOverride]); userOverride != "" {
		return MergeDeploymentOverride(mustJSON(overlay), userOverride)
	}
	return mustJSON(overlay)
}

// MergeDeploymentOverride deep-merges Magento MAGENTO_DC__OVERRIDE JSON objects.
func MergeDeploymentOverride(base, overlay string) string {
	merged := map[string]any{}
	if strings.TrimSpace(base) != "" {
		_ = json.Unmarshal([]byte(base), &merged)
	}
	var extra map[string]any
	if strings.TrimSpace(overlay) != "" {
		_ = json.Unmarshal([]byte(overlay), &extra)
	}
	if len(extra) == 0 {
		return mustJSON(merged)
	}
	return mustJSON(mergeJSONMaps(merged, extra))
}

func magentoCORSOriginList(overlays MagentoOverlays) string {
	seen := map[string]struct{}{}
	var origins []string
	for _, origin := range append(append([]string(nil), overlays.CORSOrigins...), overlays.StorefrontOrigin) {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return strings.Join(origins, ",")
}

func magentoCronConsumersRun(mode string) (string, bool) {
	switch strings.TrimSpace(mode) {
	case "cron", "both":
		return "1", true
	case "processes":
		return "0", true
	default:
		return "", false
	}
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func mergeJSONMaps(base, overlay map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for key, value := range overlay {
		existing, ok := base[key]
		if ok {
			existingMap, existingOK := existing.(map[string]any)
			overlayMap, overlayOK := value.(map[string]any)
			if existingOK && overlayOK {
				base[key] = mergeJSONMaps(existingMap, overlayMap)
				continue
			}
		}
		base[key] = value
	}
	return base
}

func mustJSON(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
