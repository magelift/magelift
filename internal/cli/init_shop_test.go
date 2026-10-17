package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
	configPath := filepath.Join(dir, "magelift.yaml")
	var out strings.Builder
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("magelift.yaml should not exist: %v", statErr)
	}
}

func TestValidateShopRefusesUnsupportedPHPBranch(t *testing.T) {
	shop := inspectedShop{Edition: "open-source", Version: "2.4.8-p5"}
	err := validateShopForInitRecipe(shop)
	if err == nil || !strings.Contains(err.Error(), "PHP 8.5") {
		t.Fatalf("err = %v", err)
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
	command.SetArgs([]string{"--config", filepath.Join(dir, "magelift.yaml"), "--output", "json", "init"})
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
	destination := filepath.Join(dir, "magelift.yaml")
	command.SetArgs([]string{"--config", destination, "init"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected regular-file refusal")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("output should remain absent: %v", err)
	}
}

func TestCommerceInitUsesCredentialReferenceWithoutReadingASecret(t *testing.T) {
	starter, err := renderMinimalStarter("gcp", inspectedShop{ProjectName: "commerce-shop", Edition: "commerce", Version: "2.4.9"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(starter, "credentials: env://COMPOSER_AUTH") {
		t.Fatal("Commerce requires a build credential reference")
	}
	if err := validateInitStarter([]byte(starter)); err != nil {
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
			path := filepath.Join(dir, "magelift.yaml")
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
