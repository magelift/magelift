package platform

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

func TestRenderLifecycleCommandKeepsInlinePHPInOneShellArgument(t *testing.T) {
	payload := "a'b " + "$(printf injected) ; " + "`printf injected`\n# comment"
	command := "php() { test \"$#\" -eq 2 && test \"$1\" = -r && printf '%s' \"$2\"; }; " + renderLifecycleCommand([]string{"php", "-r", payload})
	got, err := exec.Command("/bin/sh", "-ec", command).CombinedOutput()
	if err != nil {
		t.Fatalf("shell: %v: %s", err, got)
	}
	if string(got) != payload {
		t.Fatalf("shell changed inline PHP: %q", got)
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
		want = append(want, renderLifecycleCommand(command))
	}
	if shell[2] != strings.Join(want, " && ") {
		t.Fatalf("migration shell = %q, want golden render %q", shell[2], strings.Join(want, " && "))
	}
	if strings.Contains(shell[2], "setup:static-content:deploy") {
		t.Fatalf("migration shell = %q, want no deploy-time static-content deploy", shell[2])
	}
	bootstrap := strings.Index(shell[2], "setup:db-schema:upgrade")
	upgrade := strings.Index(shell[2], "setup:upgrade")
	importCmd := strings.Index(shell[2], "app:config:import")
	if bootstrap < 0 || upgrade < 0 || importCmd < 0 || bootstrap > upgrade || upgrade > importCmd {
		t.Fatalf("migration shell = %q, want initial database bootstrap before setup:upgrade before app:config:import", shell[2])
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

func TestProbeSearchReachabilityURLBareHostnameUsesOpenSearchPort(t *testing.T) {
	got := probeSearchReachabilityURL("shop-test-search")
	want := "http://shop-test-search:9200"
	if got != want {
		t.Fatalf("probeSearchReachabilityURL() = %q, want %q", got, want)
	}
}

func TestProbeSearchReachabilityURLExplicitSchemesUnchanged(t *testing.T) {
	cases := map[string]string{
		"https://search.internal:9200":      "https://search.internal:9200",
		"http://search.internal:9200":       "http://search.internal:9200",
		"https://search.example.com":        "https://search.example.com",
		"https://search.internal:9200/_cat": "https://search.internal:9200/_cat",
	}
	for endpoint, want := range cases {
		if got := probeSearchReachabilityURL(endpoint); got != want {
			t.Errorf("probeSearchReachabilityURL(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

func TestProbeSearchReachabilityURLBareHostPortAndAWSManaged(t *testing.T) {
	if got := probeSearchReachabilityURL("search.internal:9300"); got != "http://search.internal:9300" {
		t.Fatalf("custom port = %q", got)
	}
	managed := "vpc-shop-xxxxx.eu-west-1.es.amazonaws.com"
	if got := probeSearchReachabilityURL(managed); got != "https://"+managed+":443" {
		t.Fatalf("managed domain = %q", got)
	}
}

func TestProbeSearchHostBareEndpointsExcludePort(t *testing.T) {
	if got := probeSearchHost("search.internal:9300"); got != "search.internal" {
		t.Fatalf("probeSearchHost() = %q, want hostname without port", got)
	}
	if got := probeSearchHost("https://search.internal:9200"); got != "search.internal" {
		t.Fatalf("probeSearchHost(https) = %q", got)
	}
}

func TestMagentoProbeShellReachabilityCurlURL(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		reach    bool
		wantURL  string
	}{
		{
			name:     "bare hostname",
			endpoint: "shop-test-search",
			reach:    true,
			wantURL:  "http://shop-test-search:9200",
		},
		{
			name:     "bare host custom port",
			endpoint: "search.internal:9300",
			reach:    true,
			wantURL:  "http://search.internal:9300",
		},
		{
			name:     "explicit https",
			endpoint: "https://search.internal:9200",
			reach:    true,
			wantURL:  "https://search.internal:9200",
		},
		{
			name:     "explicit http",
			endpoint: "http://search.internal:9200",
			reach:    true,
			wantURL:  "http://search.internal:9200",
		},
		{
			name:     "https default port unchanged",
			endpoint: "https://search.example.com",
			reach:    true,
			wantURL:  "https://search.example.com",
		},
		{
			name:     "aws managed bare",
			endpoint: "vpc-shop-xxxxx.eu-west-1.es.amazonaws.com",
			reach:    true,
			wantURL:  "https://vpc-shop-xxxxx.eu-west-1.es.amazonaws.com:443",
		},
		{
			name:     "sigv4 wiring only",
			endpoint: "shop-test-search",
			reach:    false,
			wantURL:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			curlURL, ranCurl := runProbeShellRecordingCurl(t, tc.endpoint, tc.reach, "opensearch", probeSearchHost(tc.endpoint))
			if tc.wantURL == "" {
				if ranCurl {
					t.Fatalf("expected no curl, got URL %q", curlURL)
				}
				return
			}
			if !ranCurl {
				t.Fatal("expected curl to run")
			}
			if curlURL != tc.wantURL {
				t.Fatalf("curl URL = %q, want %q", curlURL, tc.wantURL)
			}
		})
	}
}

func runProbeShellRecordingCurl(t *testing.T, searchEndpoint string, checkReachability bool, engine string, magentoSearchHost string) (curlURL string, ranCurl bool) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	magento := `#!/bin/sh
case "$1" in
setup:db:status) exit 0 ;;
config:show)
  case "$2" in
  catalog/search/engine) printf '%s\n' opensearch; exit 0 ;;
  catalog/search/opensearch_server_hostname) printf '%s\n' '` + strings.ReplaceAll(magentoSearchHost, "'", "'\\''") + `'; exit 0 ;;
  esac
  ;;
esac
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "magento"), []byte(magento), 0o755); err != nil {
		t.Fatal(err)
	}
	curlLog := filepath.Join(root, "curl-url.txt")
	curlStub := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
  http://*|https://*) printf '%s' "$arg" > '` + curlLog + `'; exit 0 ;;
  esac
done
exit 1
`
	if err := os.WriteFile(filepath.Join(root, "curl"), []byte(curlStub), 0o755); err != nil {
		t.Fatal(err)
	}
	shell := MagentoProbeShell(searchEndpoint, checkReachability, engine)
	cmd := exec.Command(shell[0], shell[1], shell[2])
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := cmd.Run(); err != nil {
		t.Fatalf("probe shell: %v", err)
	}
	data, err := os.ReadFile(curlLog)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data)), true
}
