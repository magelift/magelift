package platform

import (
	"reflect"
	"strings"
	"testing"
)

func TestMagentoQueueArgs(t *testing.T) {
	want := []string{"bin/magento", "queue:consumers:start", "async.operations.all", "--max-messages=10000"}
	if got := MagentoQueueArgs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("MagentoQueueArgs() = %#v, want %#v", got, want)
	}
}

func TestMagentoQueueArgsForNamedConsumers(t *testing.T) {
	want := []string{"bin/magento", "queue:consumers:start", "product_action_attribute.update", "--max-messages=10000"}
	if got := MagentoQueueArgsFor([]string{"product_action_attribute.update"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("MagentoQueueArgsFor() = %#v, want %#v", got, want)
	}
}

func TestMagentoMigrationShellMatchesPHPDeploySequence(t *testing.T) {
	joined := strings.Join(MagentoMigrationShell(), " ")
	for _, want := range []string{"app:config:import", "setup:upgrade --keep-generated", "cache:clean", "cache:flush"} {
		if !strings.Contains(joined, want) {
			t.Errorf("migration shell = %q, want %q", joined, want)
		}
	}
	if strings.Contains(joined, "setup:static-content:deploy") {
		t.Errorf("migration shell = %q, want no deploy-time static-content deploy", joined)
	}
}

func TestMagentoProbeShellAlwaysChecksDatabaseStatus(t *testing.T) {
	joined := strings.Join(MagentoProbeShell("", false), " ")
	if !strings.Contains(joined, "setup:db:status") {
		t.Fatalf("probe shell = %q, want setup:db:status", joined)
	}
	if strings.Contains(joined, "config:show") || strings.Contains(joined, "curl") {
		t.Fatalf("probe shell without search = %q, want no search legs", joined)
	}
}

func TestMagentoProbeShellSearchLegs(t *testing.T) {
	withReachability := strings.Join(MagentoProbeShell("https://search.internal:9200", true), " ")
	for _, want := range []string{"config:show catalog/search/engine", "curl -fsS", "https://search.internal:9200"} {
		if !strings.Contains(withReachability, want) {
			t.Errorf("reachable probe = %q, want %q", withReachability, want)
		}
	}
	wiringOnly := strings.Join(MagentoProbeShell("https://search.internal:443", false), " ")
	if !strings.Contains(wiringOnly, "config:show catalog/search/engine") {
		t.Errorf("wiring-only probe = %q, want the wiring check", wiringOnly)
	}
	if strings.Contains(wiringOnly, "curl -fsS") {
		t.Errorf("wiring-only probe = %q, want no unsigned reachability leg", wiringOnly)
	}
}
