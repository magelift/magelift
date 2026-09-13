package webruntime

import (
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestRegisterDefaultsExposesNginxAndAdobeFlags(t *testing.T) {
	registry := defaults(t)
	runtime, err := registry.Get("nginx-fpm")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := runtime.Descriptor()
	if descriptor.ID != sdk.WebRuntimeNginxFPM || descriptor.Adobe != sdk.AdobeSupported || descriptor.Version == "" {
		t.Fatalf("nginx-fpm descriptor = %#v", descriptor)
	}
	if descriptor.LocalCompose.ImageFamily != "php-runtime" || descriptor.Cloud.Placement != sdk.WebRuntimePlacementSidecar {
		t.Fatalf("nginx-fpm contract = %#v", descriptor)
	}

	listed := registry.List()
	if len(listed) != 3 {
		t.Fatalf("listed %d plugins, want 3: %#v", len(listed), listed)
	}
	byID := map[sdk.WebRuntimeID]sdk.WebRuntimeDescriptor{}
	for _, item := range listed {
		byID[item.ID] = item
		if item.Version == "" || len(item.MagentoReleases) == 0 {
			t.Fatalf("plugin %q missing version or Magento range: %#v", item.ID, item)
		}
	}
	if byID[sdk.WebRuntimeNginxFPM].Adobe != sdk.AdobeSupported {
		t.Fatalf("nginx-fpm Adobe flag = %q", byID[sdk.WebRuntimeNginxFPM].Adobe)
	}
	if byID[sdk.WebRuntimeFrankenPHPClassic].Adobe != sdk.AdobeUnsupported {
		t.Fatalf("frankenphp-classic Adobe flag = %q", byID[sdk.WebRuntimeFrankenPHPClassic].Adobe)
	}
	if byID[sdk.WebRuntimePHPApache].Adobe != sdk.AdobeUnsupported {
		t.Fatalf("php-apache Adobe flag = %q", byID[sdk.WebRuntimePHPApache].Adobe)
	}
}

func TestUnknownAndWorkerIDsFailClosedNamingThePlugin(t *testing.T) {
	registry := defaults(t)
	_, err := registry.Get("community-caddy")
	if err == nil || !strings.Contains(err.Error(), "community-caddy") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unknown ID: %v", err)
	}
	_, err = registry.Admit(string(sdk.WebRuntimeFrankenPHPWorker), "2.4.9", true)
	if err == nil || !strings.Contains(err.Error(), "frankenphp-worker") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered worker: %v", err)
	}
}

func TestFrankenPHPAndApacheRequireAdobeHatch(t *testing.T) {
	registry := defaults(t)
	for _, id := range []string{"frankenphp-classic", "php-apache"} {
		_, err := registry.Admit(id, "2.4.9", false)
		if err == nil || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "Adobe-unsupported") {
			t.Fatalf("%s without hatch: %v", id, err)
		}
		warnings, err := registry.Admit(id, "2.4.9", true)
		if err != nil {
			t.Fatalf("%s with hatch: %v", id, err)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "Adobe-unsupported") || !strings.Contains(warnings[0], "MageLift-community") {
			t.Fatalf("%s hatch warnings = %v", id, warnings)
		}
		if strings.Contains(warnings[0], "Adobe-supported") || strings.Contains(warnings[0], "MageLift-certified") {
			t.Fatalf("%s hatch claimed support or certification: %q", id, warnings[0])
		}
	}
	warnings, err := registry.Admit("nginx-fpm", "2.4.9", false)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("nginx-fpm admit: warnings=%v err=%v", warnings, err)
	}
	_, err = registry.Admit("frankenphp-classic", "2.4.8-p3", false)
	if err == nil || !strings.Contains(err.Error(), "Adobe-unsupported") {
		t.Fatalf("frankenphp on 2.4.8-p3 without hatch: %v", err)
	}
}

func TestNormalizeIDDefaultsToNginxFPM(t *testing.T) {
	if got := NormalizeID(""); got != DefaultID {
		t.Fatalf("empty ID = %q", got)
	}
	if got := NormalizeID("  php-apache  "); got != "php-apache" {
		t.Fatalf("trimmed ID = %q", got)
	}
	registry := defaults(t)
	runtime, err := registry.Get("")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Descriptor().ID != sdk.WebRuntimeNginxFPM {
		t.Fatalf("empty Get ID = %q", runtime.Descriptor().ID)
	}
}

func TestLoadFromPathRefusesUnsignedWorkingDirectoryFiles(t *testing.T) {
	registry := NewRegistry()
	path := filepath.Join(".", "frankenphp-classic.so")
	err := registry.LoadFromPath(path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "unsigned") {
		t.Fatalf("unsigned file: %v", err)
	}
}

func defaults(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := RegisterDefaults(registry); err != nil {
		t.Fatal(err)
	}
	return registry
}
