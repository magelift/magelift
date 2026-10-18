package webruntime

import (
	"strings"
	"testing"
)

func TestAdmitDefaultNginx(t *testing.T) {
	warning, err := Admit("", "2.4.9", false)
	if err != nil || warning != "" {
		t.Fatalf("nginx default: warning=%q err=%v", warning, err)
	}
}

func TestAdmitUnknownID(t *testing.T) {
	_, err := Admit("caddy", "2.4.9", true)
	if err == nil || !strings.Contains(err.Error(), "caddy") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("error = %v, want missing plugin named", err)
	}
}

func TestAdmitWorkerUnregistered(t *testing.T) {
	_, err := Admit("frankenphp-worker", "2.4.9", true)
	if err == nil || !strings.Contains(err.Error(), "frankenphp-worker") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("error = %v, want missing plugin", err)
	}
}

func TestAdmitHatchFailClosed(t *testing.T) {
	for _, id := range []string{"frankenphp-classic", "php-apache"} {
		_, err := Admit(id, "2.4.9", false)
		if err == nil || !strings.Contains(err.Error(), "allowUnsupported") || !strings.Contains(err.Error(), "Adobe-unsupported") {
			t.Fatalf("%s error = %v", id, err)
		}
	}
}

func TestAdmitHatchWarns(t *testing.T) {
	for _, id := range []string{"frankenphp-classic", "php-apache"} {
		warning, err := Admit(id, "2.4.9", true)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !strings.Contains(warning, "Adobe-unsupported") || !strings.Contains(warning, "MageLift-community") || !strings.Contains(warning, id) {
			t.Fatalf("%s warning = %q", id, warning)
		}
		if strings.Contains(warning, "Adobe-supported") || strings.Contains(warning, "MageLift-certified") {
			t.Fatalf("%s hatch claimed support or certification: %q", id, warning)
		}
	}
}

func TestListFirstPartyRuntimes(t *testing.T) {
	got := List()
	if len(got) != 3 {
		t.Fatalf("list = %#v", got)
	}
	ids := map[string]ListedPlugin{}
	for _, item := range got {
		ids[item.ID] = item
		if item.Version == "" || len(item.MagentoReleases) == 0 {
			t.Fatalf("%s missing version or Magento range: %#v", item.ID, item)
		}
	}
	if !ids["nginx-fpm"].AdobeSupported {
		t.Fatal("nginx-fpm must be Adobe-supported")
	}
	if ids["frankenphp-classic"].AdobeSupported {
		t.Fatal("frankenphp-classic must not claim Adobe support")
	}
	if ids["php-apache"].AdobeSupported {
		t.Fatal("php-apache must not claim Adobe support")
	}
	for _, id := range []string{"nginx-fpm", "frankenphp-classic", "php-apache"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing %s", id)
		}
	}
	if _, ok := ids["frankenphp-worker"]; ok {
		t.Fatal("frankenphp-worker must stay unregistered")
	}
}
