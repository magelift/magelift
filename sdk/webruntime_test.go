package sdk

import (
	"strings"
	"testing"
)

func TestValidateWebRuntimeDescriptorAcceptsNginxWithoutCoreOutputKeys(t *testing.T) {
	descriptor := nginxWebRuntimeDescriptor()
	if err := ValidateWebRuntimeDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExtensionDescriptor(ExtensionDescriptor{
		APIVersion: descriptor.APIVersion,
		ID:         string(descriptor.ID),
		Version:    descriptor.Version,
		Source:     descriptor.Source,
		Tier:       descriptor.Tier,
	}); err == nil || !strings.Contains(err.Error(), "output key") {
		t.Fatalf("web-runtime descriptors must not require stack CoreOutputKeys: %v", err)
	}
}

func TestValidateWebRuntimeDescriptorRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*WebRuntimeDescriptor)
		contain string
	}{
		{name: "api", mutate: func(d *WebRuntimeDescriptor) { d.APIVersion = "v0" }, contain: "API version"},
		{name: "source", mutate: func(d *WebRuntimeDescriptor) { d.Source = "" }, contain: "source"},
		{name: "adobe", mutate: func(d *WebRuntimeDescriptor) { d.Adobe = "certified" }, contain: "Adobe"},
		{name: "placement", mutate: func(d *WebRuntimeDescriptor) { d.Cloud.Placement = "task" }, contain: "placement"},
		{name: "health", mutate: func(d *WebRuntimeDescriptor) { d.LocalCompose.HealthPath = "health" }, contain: "health path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			descriptor := nginxWebRuntimeDescriptor()
			test.mutate(&descriptor)
			err := ValidateWebRuntimeDescriptor(descriptor)
			if err == nil || !strings.Contains(err.Error(), test.contain) {
				t.Fatalf("got %v, want substring %q", err, test.contain)
			}
		})
	}
}

func TestAdobeSupportForSeparatesNginxFromCommunityServers(t *testing.T) {
	if got := AdobeSupportFor(WebRuntimeNginxFPM, "2.4.9"); got != AdobeSupported {
		t.Fatalf("nginx-fpm Adobe support = %q", got)
	}
	if got := AdobeSupportFor(WebRuntimeFrankenPHPClassic, "2.4.9"); got != AdobeUnsupported {
		t.Fatalf("frankenphp-classic Adobe support = %q", got)
	}
	if got := AdobeSupportFor(WebRuntimePHPApache, "2.4.9"); got != AdobeUnsupported {
		t.Fatalf("php-apache Adobe support on 2.4.9 = %q", got)
	}
	if got := AdobeSupportFor(WebRuntimePHPApache, "2.4.8-p3"); got != AdobeUnsupported {
		t.Fatalf("php-apache Adobe support on 2.4.8-p3 = %q", got)
	}
	if got := AdobeSupportFor(WebRuntimeFrankenPHPWorker, "2.4.8-p5"); got != AdobeUnsupported {
		t.Fatalf("frankenphp-worker Adobe support = %q", got)
	}
}

func TestAdmitWebRuntimeAppliesAdobeHatch(t *testing.T) {
	warnings, err := AdmitWebRuntime(WebRuntimeNginxFPM, "2.4.9", false)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("nginx-fpm admit: warnings=%v err=%v", warnings, err)
	}
	if _, err := AdmitWebRuntime(WebRuntimeFrankenPHPClassic, "2.4.9", false); err == nil || !strings.Contains(err.Error(), "frankenphp-classic") || !strings.Contains(err.Error(), "Adobe-unsupported") {
		t.Fatalf("frankenphp without hatch: %v", err)
	}
	warnings, err = AdmitWebRuntime(WebRuntimeFrankenPHPClassic, "2.4.9", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Adobe-unsupported") || !strings.Contains(warnings[0], "MageLift-community") {
		t.Fatalf("frankenphp hatch warnings = %v", warnings)
	}
	if strings.Contains(warnings[0], "Adobe-supported") || strings.Contains(warnings[0], "MageLift-certified") {
		t.Fatalf("hatch warning claimed Adobe support or certification: %q", warnings[0])
	}
}

func nginxWebRuntimeDescriptor() WebRuntimeDescriptor {
	return WebRuntimeDescriptor{
		APIVersion:      ExtensionAPIVersion,
		ID:              WebRuntimeNginxFPM,
		Version:         "0.1.0",
		Source:          "magelift",
		Tier:            ExtensionTierExperimental,
		Adobe:           AdobeSupported,
		MagentoReleases: []string{"2.4.6", "2.4.7", "2.4.8", "2.4.9"},
		LocalCompose:    WebRuntimeComposeHints{ImageFamily: "php-runtime", HealthPath: WebRuntimeHealthPath},
		Cloud:           WebRuntimeCloudHints{Ports: []int{8080}, Placement: WebRuntimePlacementSidecar},
	}
}
