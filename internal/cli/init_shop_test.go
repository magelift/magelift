package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/projectphp"
)

func TestInitNewCreatesSelectedComposerProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exercises a POSIX Composer stand-in")
	}
	bin := t.TempDir()
	composer := `#!/bin/sh
set -eu
test "$1" = create-project && test "$2" = --no-interaction
test "$3" = --repository-url=https://repo.magento.com/
test "$4" = "$EXPECTED_PACKAGE" && test "$6" = 2.4.8-p1
cp -R "$SHOP_FIXTURE/." "$5/"
exit "$COMPOSER_STATUS"
`
	if err := os.WriteFile(filepath.Join(bin, "composer"), []byte(composer), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct{ edition, pkg, status string }{
		{"open-source", "magento/project-community-edition", "0"},
		{"commerce", "magento/project-enterprise-edition", "0"},
		{"open-source", "magento/project-community-edition", "1"},
	} {
		t.Run(tc.edition+tc.status, func(t *testing.T) {
			dir, fixture := t.TempDir(), t.TempDir()
			writeInitShopFixture(t, fixture, "2.4.8-p1", tc.edition)
			t.Setenv("EXPECTED_PACKAGE", tc.pkg)
			t.Setenv("SHOP_FIXTURE", fixture)
			t.Setenv("COMPOSER_STATUS", tc.status)
			path := filepath.Join(dir, "magelift.php")
			var output bytes.Buffer
			cmd := newCommand(&output, &output, nil)
			cmd.SetArgs([]string{"--config", path, "init", "--new", "--edition", tc.edition, "--magento-version", "2.4.8-p1"})
			err := cmd.Execute()
			if tc.status == "1" {
				files, readErr := os.ReadDir(dir)
				if err == nil || readErr != nil || len(files) != 0 {
					t.Fatalf("failed installation left %v, err=%v, read=%v", files, err, readErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := projectphp.Evaluate(t.Context(), path)
			if err != nil || !strings.Contains(string(got), `"edition":"`+tc.edition+`"`) || !strings.Contains(string(got), "2.4.8-p1") {
				t.Fatalf("created project mismatch: %s, %v", got, err)
			}
		})
	}
}

func TestInitProjectNameFitsGCPResourceContract(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	dir := t.TempDir()
	path := filepath.Join(dir, "composer.json")
	seen := map[string]bool{}
	for _, input := range []string{"vendor/001-shop", "vendor/" + strings.Repeat("a", 50) + "-one", "vendor/" + strings.Repeat("a", 50) + "-two", "vendor/normal-shop"} {
		data, err := json.Marshal(composerProject{Name: input})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		name, err := projectNameFromComposer(path)
		if err != nil || !valid.MatchString(name) || seen[name] {
			t.Fatalf("input=%q name=%q duplicate=%v error=%v", input, name, seen[name], err)
		}
		again, err := projectNameFromComposer(path)
		if err != nil || again != name {
			t.Fatalf("unstable name: %q then %q, error=%v", name, again, err)
		}
		if input == "vendor/normal-shop" && name != "normal-shop" {
			t.Fatalf("ordinary name changed: %q", name)
		}
		seen[name] = true
	}
}

func TestInspectShopSourceFromLock(t *testing.T) {
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "community")

	shop, err := inspectShopSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	if shop.Edition != "open-source" || shop.Version != "2.4.9" {
		t.Fatalf("shop = %#v", shop)
	}
	if shop.ProjectName != "example-shop" {
		t.Fatalf("project name = %q", shop.ProjectName)
	}
}

func TestInspectShopSourceMissingLeavesAbsent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.php")
	var out strings.Builder
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("magelift.php should not exist: %v", statErr)
	}
}

func TestShopStarterSelectsRecommendedPHP(t *testing.T) {
	for _, tc := range []struct{ version, php string }{{"2.4.7-p10", "8.3"}, {"2.4.8-p5", "8.4"}, {"2.4.9", "8.5"}} {
		shop := inspectedShop{ProjectName: "test-shop", Edition: "open-source", Version: tc.version}
		if err := validateShopForInitRecipe(shop); err != nil {
			t.Fatal(err)
		}
		starter, err := starterDocument("gcp", shop)
		if err != nil || starter["build"].(map[string]any)["php"] != tc.php {
			t.Fatalf("starter for %s = %v, error = %v", tc.version, starter, err)
		}
	}
}

func TestStarterChoosesAvailableValkeyEightSeries(t *testing.T) {
	shop := inspectedShop{ProjectName: "test-shop", Edition: "open-source", Version: "2.4.8-p5"}
	for provider, want := range map[string]string{"gcp": "8.0", "aws": "8.1"} {
		starter, err := starterDocument(provider, shop)
		if err != nil {
			t.Fatal(err)
		}
		if got := starter["services"].(map[string]any)["cache"].(map[string]any)["version"]; got != want {
			t.Errorf("%s cache version = %v, want %s", provider, got, want)
		}
	}
}

func TestInitCreatesCurrentAdobePatchTemplates(t *testing.T) {
	for _, tc := range []struct{ version, php string }{{"2.4.7-p10", "8.3"}, {"2.4.8-p5", "8.4"}, {"2.4.9", "8.5"}} {
		t.Run(tc.version, func(t *testing.T) {
			dir := t.TempDir()
			writeInitShopFixture(t, dir, tc.version, "community")
			path := filepath.Join(dir, "magelift.php")
			var output strings.Builder
			cmd := newCommand(&output, &output, nil)
			cmd.SetArgs([]string{"--config", path, "init"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			data, err := projectphp.Evaluate(t.Context(), path)
			if err != nil || !strings.Contains(string(data), `"php":"`+tc.php+`"`) {
				t.Fatalf("generated starter = %s, error = %v", data, err)
			}
		})
	}
}

func writeInitShopFixture(t *testing.T, dir, version, edition string) {
	t.Helper()
	pkg := "magento/product-community-edition"
	if edition == "commerce" {
		pkg = "magento/product-enterprise-edition"
	}
	lock := `{"packages":[{"name":"` + pkg + `","version":"` + version + `"}]}`
	compose := `{"name":"example/example-shop","type":"project"}`
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"composer.lock": lock,
		"composer.json": compose,
		"bin/magento":   "#!/usr/bin/env php\n<?php\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInitSourceCheckoutUsesRootReleaseWithoutChangingShop(t *testing.T) {
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "community")
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"name":"magento/magento2ce","version":"2.4.9"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(`{"packages":[{"name":"symfony/console","version":"v7.4.21"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	command := newCommand(&output, &output, nil)
	command.SetArgs([]string{"--config", filepath.Join(dir, "magelift.php"), "--output", "json", "init"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var result initWriteResult
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if result.Release != "2.4.9" || result.Edition != "open-source" {
		t.Fatalf("result = %+v", result)
	}
}

func TestInitPrefersLockedCommerceEditionOverCommunityDependency(t *testing.T) {
	edition, version, err := magentoReleaseFromLock([]byte(`{"packages":[{"name":"magento/product-community-edition","version":"2.4.9"},{"name":"magento/product-enterprise-edition","version":"2.4.9"}]}`))
	if err != nil || edition != "commerce" || version != "2.4.9" {
		t.Fatalf("edition=%s version=%s error=%v", edition, version, err)
	}
}

func TestInitRejectsSymlinkedRequiredSourceWithoutWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Unix permissions")
	}
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "community")
	original := filepath.Join(dir, "composer.lock")
	target := filepath.Join(t.TempDir(), "composer.lock")
	if err := os.Rename(original, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, original); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	command := newCommand(&output, &output, nil)
	destination := filepath.Join(dir, "magelift.php")
	command.SetArgs([]string{"--config", destination, "init"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected regular-file refusal")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("output should remain absent: %v", err)
	}
}

func TestCommerceInitUsesCredentialReferenceWithoutReadingASecret(t *testing.T) {
	starter, err := starterDocument("gcp", inspectedShop{ProjectName: "commerce-shop", Edition: "commerce", Version: "2.4.9"})
	if err != nil {
		t.Fatal(err)
	}
	if starter["build"].(map[string]any)["composer"].(map[string]any)["credentials"] != "env://COMPOSER_AUTH" {
		t.Fatal("Commerce requires a build credential reference")
	}
	b, err := json.Marshal(starter)
	if err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.ResolveBuild(); err != nil {
		t.Fatal(err)
	}
}

func TestInitRejectsNullComposerMetadataBeforeWriting(t *testing.T) {
	for _, file := range []string{"composer.json", "composer.lock"} {
		t.Run(file, func(t *testing.T) {
			dir := t.TempDir()
			writeInitShopFixture(t, dir, "2.4.9", "community")
			if err := os.WriteFile(filepath.Join(dir, file), []byte("null"), 0o600); err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			command := newCommand(&output, &output, nil)
			path := filepath.Join(dir, "magelift.php")
			command.SetArgs([]string{"--config", path, "init"})
			if err := command.Execute(); err == nil {
				t.Fatal("expected object requirement")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("output should remain absent: %v", err)
			}
		})
	}
}
