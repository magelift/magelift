package platform

import (
	"encoding/json"
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

func TestMagentoMigrationShellRendersGoldenSequence(t *testing.T) {
	shell := MagentoMigrationShell()
	if len(shell) != 3 || shell[0] != "/bin/sh" || shell[1] != "-ec" {
		t.Fatalf("migration shell = %#v", shell)
	}
	var golden struct {
		Deploy     [][]string `json:"deploy"`
		PostDeploy [][]string `json:"postDeploy"`
	}
	if err := json.Unmarshal(lifecycleDeployGolden, &golden); err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, command := range append(append([][]string{}, golden.Deploy...), golden.PostDeploy...) {
		want = append(want, strings.Join(command, " "))
	}
	if shell[2] != strings.Join(want, " && ") {
		t.Fatalf("migration shell = %q, want golden render %q", shell[2], strings.Join(want, " && "))
	}
	if strings.Contains(shell[2], "setup:static-content:deploy") {
		t.Fatalf("migration shell = %q, want no deploy-time static-content deploy", shell[2])
	}
	upgrade := strings.Index(shell[2], "setup:upgrade")
	importCmd := strings.Index(shell[2], "app:config:import")
	if upgrade < 0 || importCmd < 0 || upgrade > importCmd {
		t.Fatalf("migration shell = %q, want setup:upgrade before app:config:import", shell[2])
	}
}

func TestMagentoProbeShellAlwaysChecksDatabaseStatus(t *testing.T) {
	joined := strings.Join(MagentoProbeShell("", false, ""), " ")
	if !strings.Contains(joined, "setup:db:status") {
		t.Fatalf("probe shell = %q, want setup:db:status", joined)
	}
	if strings.Contains(joined, "config:show") || strings.Contains(joined, "curl") {
		t.Fatalf("probe shell without search = %q, want no search legs", joined)
	}
}

func TestMagentoProbeShellSearchLegs(t *testing.T) {
	withReachability := strings.Join(MagentoProbeShell("https://search.internal:9200", true, ""), " ")
	for _, want := range []string{"config:show catalog/search/engine", "curl -fsS", "https://search.internal:9200"} {
		if !strings.Contains(withReachability, want) {
			t.Errorf("reachable probe = %q, want %q", withReachability, want)
		}
	}
	wiringOnly := strings.Join(MagentoProbeShell("https://search.internal:443", false, ""), " ")
	if !strings.Contains(wiringOnly, "config:show catalog/search/engine") {
		t.Errorf("wiring-only probe = %q, want the wiring check", wiringOnly)
	}
	if strings.Contains(wiringOnly, "curl -fsS") {
		t.Errorf("wiring-only probe = %q, want no unsigned reachability leg", wiringOnly)
	}
}

func TestMagentoProbeShellAssertsEffectiveSearchConfig(t *testing.T) {
	joined := strings.Join(MagentoProbeShell("https://search.internal:9200", false, "opensearch"), " ")
	for _, want := range []string{
		`test "$(bin/magento config:show catalog/search/opensearch_server_hostname)" = 'search.internal'`,
		`test "$(bin/magento config:show catalog/search/engine)" = 'opensearch'`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("probe = %q, want %q", joined, want)
		}
	}
	unscoped := strings.Join(MagentoProbeShell("https://search.internal:9200", false, ""), " ")
	if !strings.Contains(unscoped, "opensearch_server_hostname") {
		t.Errorf("probe = %q, want host assertion without engine scope", unscoped)
	}
	if strings.Count(unscoped, "catalog/search/engine") != 1 {
		t.Errorf("probe = %q, want engine check without engine assertion", unscoped)
	}
}
