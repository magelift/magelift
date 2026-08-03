package localdev

import (
	"reflect"
	"strings"
	"testing"
)

func TestProjectNameIsStableAndBounded(t *testing.T) {
	name, err := ProjectName(" Shop / EU ")
	if err != nil {
		t.Fatal(err)
	}
	if name != "magelift-shop-eu" {
		t.Fatalf("name = %q", name)
	}
	long, err := ProjectName(strings.Repeat("x", 100))
	if err != nil {
		t.Fatal(err)
	}
	if len(long) > len("magelift-")+48 {
		t.Fatalf("bounded name length = %d", len(long))
	}
}

func TestComposeArgsUseArgumentVectors(t *testing.T) {
	got, err := ComposeArgs(".magelift/compose.local.yml", "magelift-shop", "exec", "app", []string{"bin/magento", "cache:flush"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "-f", ".magelift/compose.local.yml", "--project-name", "magelift-shop", "exec", "app", "bin/magento", "cache:flush"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	if _, err := ComposeArgs("compose.yml", "project", "exec", "app", nil); err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestComposeArgsEnableAllMagentoServicesForApp(t *testing.T) {
	got, err := ComposeArgs("compose.yml", "project", "up", "app", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "-f", "compose.yml", "--project-name", "project", "--profile", "app", "--profile", "search", "--profile", "queue", "up", "-d", "app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestComposeArgsEnableOptionalCapabilityProfiles(t *testing.T) {
	for _, service := range []string{"search", "queue"} {
		got, err := ComposeArgs("compose.yml", "project", "up", service, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"compose", "-f", "compose.yml", "--project-name", "project", "--profile", service, "up", "-d", service}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("service %q args = %#v, want %#v", service, got, want)
		}
	}
}

func TestComposeTemplateIncludesMagentoCapabilityServices(t *testing.T) {
	for _, service := range []string{"search:", "queue:", "env_file:", "local.env", "opensearchproject/opensearch:3@sha256:", "rabbitmq:4.2-management@sha256:", "MAGENTO_SEARCH_HOST: search", "MAGENTO_QUEUE_HOST: queue", "MAGELIFT_LOCAL_HTTPS_PORT:-8443", "valkey-cli", "condition: service_healthy"} {
		if !strings.Contains(ComposeTemplate, service) {
			t.Fatalf("ComposeTemplate does not contain %q", service)
		}
	}
}

func TestComposeAppWaitsForCapabilityHealth(t *testing.T) {
	parts := strings.SplitN(ComposeTemplate, "  app:\n", 2)
	if len(parts) != 2 {
		t.Fatal("ComposeTemplate does not contain the app service")
	}
	app := parts[1]
	for _, dependency := range []string{"database", "cache", "search", "queue"} {
		if !strings.Contains(app, "      "+dependency+":\n        condition: service_healthy") {
			t.Fatalf("app does not wait for healthy %s", dependency)
		}
	}
}

func TestComposeArgsRequireKnownActions(t *testing.T) {
	for _, action := range []string{"up", "down", "reset", "status", "logs"} {
		if _, err := ComposeArgs("compose.yml", "project", action, "", nil); err != nil {
			t.Fatalf("action %q: %v", action, err)
		}
	}
	if _, err := ComposeArgs("compose.yml", "project", "unknown", "", nil); err == nil {
		t.Fatal("expected unknown action error")
	}
}
