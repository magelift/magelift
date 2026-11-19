package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	buildcontext "github.com/magelift/magelift"
	"github.com/magelift/magelift/internal/toolchain"
)

func TestComposerAdvisoryLookup(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"clean", `{}`, ""},
		{"high", `{"vulns":[{"id":"GHSA-test","database_specific":{"severity":"HIGH"}}]}`, ""},
		{"critical", `{"vulns":[{"id":"GHSA-test","database_specific":{"severity":"CRITICAL"}}]}`, "critical advisory"},
		{"unknown severity", `{"vulns":[{"id":"GHSA-test"}]}`, ""},
		{"invalid", `{`, ""},
		{"paginated", `{"next_page_token":"next"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method %s", r.Method)
				}
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			var diagnostics bytes.Buffer
			err := checkComposerAdvisories(context.Background(), srv.Client(), srv.URL, "2.11.2", &diagnostics)
			if tc.name == "high" && !strings.Contains(diagnostics.String(), "high advisory GHSA-test") {
				t.Fatalf("missing high advisory warning: %q", diagnostics.String())
			}
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error %v, want %s", err, tc.want)
			}
		})
	}
}

func TestComposerVersionMatches(t *testing.T) {
	for _, tc := range []struct {
		required, actual string
		want             bool
	}{
		{"~2.9.8", "2.9.9", true}, {"~2.9.8", "2.10.0", false}, {"~2.9.8", "2.9.7", false}, {"2.10+", "2.11.2", true}, {"2.10", "2.11.2", false}, {"2.9.8+", "2.10.0", true}, {"2.9.8+", "2.9.7", false}, {"2.9", "2.9.8", true}, {"2.9.8", "2.9.9", false},
	} {
		if got := composerVersionMatches(tc.required, tc.actual); got != tc.want {
			t.Errorf("%s, %s = %t", tc.required, tc.actual, got)
		}
	}
}

func TestDockerComposerPreflight(t *testing.T) {
	if os.Getenv("MAGELIFT_TEST_DOCKER_COMPOSER") != "1" {
		t.Skip("set MAGELIFT_TEST_DOCKER_COMPOSER=1 for the Docker smoke")
	}
	source, err := buildcontext.ComposerRecipe("")
	if err != nil {
		t.Fatal(err)
	}
	sourceVersion, err := detectComposer(context.Background(), source, "linux/amd64", "/usr/bin/composer")
	if err != nil || sourceVersion != "2.11.2" {
		t.Fatalf("source Composer %s: %v", sourceVersion, err)
	}
	customDir := t.TempDir()
	dockerfile := "FROM " + source + "\nENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/sbin\n"
	if err := os.WriteFile(filepath.Join(customDir, "Dockerfile"), []byte(dockerfile), 0600); err != nil {
		t.Fatal(err)
	}
	customTag := "magelift-composer-path-smoke:local"
	t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", customTag).Run() })
	build := exec.Command("docker", "build", "--network=none", "-t", customTag, customDir)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build no-PATH source: %s: %v", out, err)
	}
	if err := exec.Command("docker", "run", "--rm", "--network=none", "--entrypoint", "/bin/sh", customTag, "-c", "! command -v composer").Run(); err != nil {
		t.Fatal("Composer unexpectedly available on PATH")
	}
	customVersion, err := detectComposer(context.Background(), customTag, "linux/amd64", "/usr/bin/composer")
	if err != nil || customVersion != sourceVersion {
		t.Fatalf("no-PATH source Composer %q: %v", customVersion, err)
	}
	images, err := toolchain.BuildImagesForPHP(context.Background(), os.Stderr, t.TempDir(), "linux/amd64", "", "8.5")
	if err != nil {
		t.Fatal(err)
	}
	builderTag := strings.Replace(images.RuntimeReference, "magelift/php-nginx:", "magelift/php-builder:", 1)
	t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", images.RuntimeReference, builderTag).Run() })
	got, err := detectBuilderComposer(context.Background(), images.BuilderID, "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2.11.2" || !composerVersionMatches("2.10+", got) || composerVersionMatches("2.10", got) {
		t.Fatalf("unexpected builder Composer %q", got)
	}
	if err := checkComposerAdvisories(context.Background(), &http.Client{Timeout: 15 * time.Second}, "https://api.osv.dev/v1/query", got, os.Stderr); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("docker", "run", "--rm", "--network=none", "--entrypoint", "php", images.RuntimeID, "-r", `echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;`).CombinedOutput()
	if err != nil || string(out) != "8.5" {
		t.Fatalf("runtime PHP = %q: %v", out, err)
	}
}

func TestComposerAdvisoryPagination(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			fmt.Fprint(w, `{"vulns":[{"id":"GHSA-unknown"}],"next_page_token":"next"}`)
			return
		}
		var query map[string]any
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil || query["page_token"] != "next" {
			t.Errorf("page token missing: %v, %v", query, err)
		}
		fmt.Fprint(w, `{"vulns":[{"id":"GHSA-critical","database_specific":{"severity":"CRITICAL"}}]}`)
	}))
	defer srv.Close()
	var diagnostics bytes.Buffer
	err := checkComposerAdvisories(context.Background(), srv.Client(), srv.URL, "2.9.8", &diagnostics)
	if err == nil || !strings.Contains(err.Error(), "GHSA-critical") || requests != 2 || !strings.Contains(diagnostics.String(), "unknown severity") {
		t.Fatalf("error %v, requests %d, diagnostics %q", err, requests, diagnostics.String())
	}
}

func TestComposerAdvisoryUnavailableWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	var diagnostics bytes.Buffer
	if err := checkComposerAdvisories(context.Background(), srv.Client(), srv.URL, "2.9.8", &diagnostics); err != nil || !strings.Contains(diagnostics.String(), "incomplete") {
		t.Fatalf("error %v, diagnostics %q", err, diagnostics.String())
	}
}

func TestComposerVersionParsingRejectsPrerelease(t *testing.T) {
	if composerBinaryVersion.MatchString("Composer version 2.11.2-RC1 2026-01-01") {
		t.Fatal("prerelease misidentified as stable")
	}
	if !composerBinaryVersion.MatchString("Composer version 2.11.2 2026-01-01") {
		t.Fatal("stable version not recognized")
	}
}

func TestComposerCVSSSeverity(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
		known      bool
	}{
		{"v3 critical", `{"severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}]}`, 4, true},
		{"v3 high", `{"severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:H/I:H/A:H"}]}`, 3, true},
		{"v4 critical", `{"severity":[{"type":"CVSS_V4","score":"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:H/SI:H/SA:H"}]}`, 4, true},
		{"maximum", `{"database_specific":{"severity":"LOW"},"severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}]}`, 4, true},
		{"invalid", `{"severity":[{"type":"CVSS_V3","score":"invalid"}]}`, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a composerAdvisory
			if err := json.Unmarshal([]byte(tc.body), &a); err != nil {
				t.Fatal(err)
			}
			got, known := advisorySeverity(a)
			if got != tc.want || known != tc.known {
				t.Fatalf("severity %d/%t, want %d/%t", got, known, tc.want, tc.known)
			}
		})
	}
}
