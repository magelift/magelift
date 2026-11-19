package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/projectphp"
)

func TestInitFromAccWritesLoadValidConfig(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "magelift.php")
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --from-acc: %v\n%s", err, out.String())
	}

	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into magelift.php")
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, data)
	}
	if len(file.Environments()) == 0 {
		t.Fatal("expected environments")
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	for _, env := range file.Environments() {
		if _, err := file.Resolve(env, config.ResolveOptions{}); err != nil {
			t.Fatalf("Resolve(%s): %v", env, err)
		}
	}

	// Same validation path as `magelift config validate`.
	var validateOut bytes.Buffer
	validate := newCommand(&validateOut, &validateOut, nil)
	validate.SetArgs([]string{"--config", configPath, "config", "validate"})
	if err := validate.Execute(); err != nil {
		t.Fatalf("config validate: %v\n%s", err, validateOut.String())
	}
}

func TestInitProviderStartersValidate(t *testing.T) {
	for _, provider := range []string{"aws", "gcp", "azure"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			writeInitShopFixture(t, dir, "2.4.9", "community")
			configPath := filepath.Join(dir, "magelift.php")
			var out bytes.Buffer
			cmd := newCommand(&out, &out, nil)
			cmd.SetArgs([]string{"--config", configPath, "init", "--provider", provider})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("init --provider %s: %v\n%s", provider, err, out.String())
			}
			var validateOut bytes.Buffer
			validate := newCommand(&validateOut, &validateOut, nil)
			validate.SetArgs([]string{"--config", configPath, "config", "validate"})
			if err := validate.Execute(); err != nil {
				t.Fatalf("config validate: %v\n%s", err, validateOut.String())
			}
			data, err := projectphp.Evaluate(t.Context(), configPath)
			if err != nil {
				t.Fatal(err)
			}
			file, err := config.Load(data)
			if err != nil {
				t.Fatal(err)
			}
			if got := file.Environments(); strings.Join(got, ",") != "preview,production,staging,uat" {
				t.Fatalf("environments = %v, want [test]", got)
			}
		})
	}
}

func TestInitProviderDefaultsToGCP(t *testing.T) {
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "community")
	configPath := filepath.Join(dir, "magelift.php")
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}
	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"provider":"gcp"`) {
		t.Fatalf("default starter is not GCP:\n%s", data)
	}
}

func TestInitUnknownProviderRefusedBeforeOverwrite(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.php")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--provider", "ovh"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown starter provider") {
		t.Fatalf("err = %v, want MageLift authority refusal", err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2", ExitCode(err))
	}
}

func TestInitFromAccRefusesExistingConfig(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	configPath := filepath.Join(dir, "magelift.php")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected refuse-if-exists")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2", ExitCode(err))
	}
}

func TestInitFromAccAndFromUpsunMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.php")
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc", "--from-upsun"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v out=%s", ExitCode(err), err, out.String())
	}
	if !strings.Contains(err.Error(), "from-acc") || !strings.Contains(err.Error(), "from-upsun") {
		t.Fatalf("error should mention both flags: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("should not write config on mutual exclusion: %v", err)
	}
}

func TestInitFromAccOverwriteWithYes(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	configPath := filepath.Join(dir, "magelift.php")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "--yes", "init", "--from-acc"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --from-acc --yes: %v\n%s", err, out.String())
	}
	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == starterConfig {
		t.Fatal("expected overwrite of existing starter config")
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("config.Load after overwrite: %v", err)
	}
}

func TestInitConfigOutWritesSideFileLeavesDefault(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	defaultPath := filepath.Join(dir, "magelift.php")
	sidePath := filepath.Join(dir, "review.magelift.php")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", defaultPath, "init", "--from-acc", "--config-out", sidePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --config-out: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		t.Fatalf("default magelift.php should remain absent: %v", err)
	}
	data, err := projectphp.Evaluate(t.Context(), sidePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("config.Load side file: %v", err)
	}
}

func TestInitConfigOutRefusesExistingWithoutYes(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	sidePath := filepath.Join(dir, "review.magelift.php")
	if err := os.WriteFile(sidePath, []byte("existing: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"init", "--from-acc", "--config-out", sidePath})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected refuse existing --config-out path")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v", ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists message, got %v", err)
	}
}

func TestInitFromUpsunAccepted(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "upsun", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	configPath := filepath.Join(dir, "magelift.php")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-upsun"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --from-upsun: %v\n%s", err, out.String())
	}
	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, data)
	}
	if _, err := os.Stat(filepath.Join(dir, "magelift.unmapped.md")); !os.IsNotExist(err) {
		t.Fatalf("supported Upsun import must not write sidecar: %v", err)
	}
}

func TestInitFromAccUnmappedWritesSidecarAndExitsNonZero(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	configPath := filepath.Join(dir, "magelift.php")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit on unmapped keys")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v", ExitCode(err), err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("YAML must still be written: %v", err)
	}
	sidecar := filepath.Join(dir, "magelift.unmapped.md")
	report, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}
	for _, want := range []string{"hooks.build", "crons.shell-cleanup", "CUSTOM_FEATURE_FLAG"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("sidecar missing %q:\n%s", want, report)
		}
	}
	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("written YAML must Load: %v", err)
	}
}

func TestInitConfigOutUnmappedSidecarUsesStem(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	writeInitShopFixture(t, dir, "2.4.8-p5", "community")
	sidePath := filepath.Join(dir, "review.magelift.php")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"init", "--from-acc", "--config-out", sidePath})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit on unmapped keys")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2", ExitCode(err))
	}
	sidecar := filepath.Join(dir, "review.magelift.unmapped.md")
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("stem sidecar missing: %v", err)
	}
}

func TestInitBareWithYesOverwritesStarter(t *testing.T) {
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "community")
	configPath := filepath.Join(dir, "magelift.php")
	old := starterConfig + "# marker\n"
	if err := os.WriteFile(configPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "--yes", "init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --yes: %v\n%s", err, out.String())
	}
	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == old {
		t.Fatal("expected overwrite of existing config")
	}
	if !strings.Contains(string(data), `"staging":`) {
		t.Fatalf("expected staging environment starter:\n%s", data)
	}
	if !strings.Contains(string(data), `"mode":"integrated"`) || !strings.Contains(string(data), `"webRuntime":"nginx-fpm"`) || !strings.Contains(string(data), `"frontName":"admin"`) {
		t.Fatalf("starter is not an integrated PHP storefront: %s", data)
	}
}

func TestShopExamplesDefineStagingAndBuildInputs(t *testing.T) {
	for _, relative := range []string{"examples/magelift.php", "examples/sample-shop/magelift.php"} {
		t.Run(relative, func(t *testing.T) {
			path := filepath.Join(cliRepoRoot(t), relative)
			data, err := projectphp.Evaluate(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			file, err := config.Load(data)
			if err != nil {
				t.Fatal(err)
			}
			spec, err := file.ResolveBuild()
			if err != nil || len(spec.Build.StaticContent.Locales) == 0 || len(spec.Build.StaticContent.Themes) == 0 {
				t.Fatalf("example build inputs = %#v, error = %v", spec.Build.StaticContent, err)
			}
			stage, err := file.Resolve("staging", config.ResolveOptions{})
			if err != nil || stage.Config.Class != "staging" {
				t.Fatalf("staging class = %q, error = %v", stage.Config.Class, err)
			}
			var out bytes.Buffer
			cmd := newCommand(&out, &out, nil)
			cmd.SetArgs([]string{"--config", path, "--output", "json", "config", "validate"})
			if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), `"valid": true`) {
				t.Fatalf("config validate: %v\n%s", err, out.String())
			}
		})
	}
}

func cliRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatalf("copyTree: %v", err)
	}
}

func TestInitGCPStarterMatchesAlphaRecipe(t *testing.T) {
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "community")
	configPath := filepath.Join(dir, "magelift.php")
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "--output", "json", "init", "--provider", "gcp"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --provider gcp: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"release": "2.4.9"`) || !strings.Contains(out.String(), `"edition": "open-source"`) {
		t.Fatalf("json init summary missing detection: %s", out.String())
	}
	data, err := projectphp.Evaluate(t.Context(), configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "\n    openSearchMode: disabled") {
		t.Fatal("starter pins search off; the alpha recipe serves OpenSearch by default")
	}
	for _, want := range []string{`"staticContent":`, `"locales":["en_US"]`, "us-central1", "staging.example.invalid", `"staging":`} {
		if !strings.Contains(text, want) {
			t.Errorf("starter missing %q", want)
		}
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", config.ResolveOptions{})
	if err != nil {
		t.Fatalf("test resolve: %v", err)
	}
	if got := effective.Config.Build.StaticContent.Locales; len(got) != 1 || got[0] != "en_US" {
		t.Fatalf("staticContent locales = %#v", got)
	}
}

func TestModernUpsunImportsToEachCloudBesideSelectedApplication(t *testing.T) {
	for _, cloud := range []string{"aws", "gcp", "azure"} {
		t.Run(cloud, func(t *testing.T) {
			root := t.TempDir()
			app := filepath.Join(root, "commerce")
			writeInitShopFixture(t, app, "2.4.8-p2", "commerce")
			dir := filepath.Join(root, ".upsun")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			input := "applications:\n  api:\n    type: php:8.3\n    source: {root: commerce}\n    relationships: {database: {service: sql}}\n  frontend: {type: 'nodejs:22'}\nservices:\n  sql: {type: 'mariadb:10.11'}\n"
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)
			var out bytes.Buffer
			cmd := newCommand(&out, &out, nil)
			cmd.SetArgs([]string{"init", "--from-upsun", "--app", "api", "--provider", cloud})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err, out.String())
			}
			data, err := projectphp.Evaluate(t.Context(), filepath.Join(app, "magelift.php"))
			if err != nil {
				t.Fatal(err)
			}
			f, err := config.Load(data)
			if err != nil {
				t.Fatal(err)
			}
			e, err := f.Resolve("staging", config.ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			db := e.Config.Services["database"]
			if db.Family != "mariadb" || db.Version != "10.11" || e.Config.Application.Edition != "commerce" || e.Config.Build.PHP != "8.3" || e.Config.Target.Provider != cloud {
				t.Fatalf("import lost destination or source metadata: %#v", db)
			}
			if _, err := os.Stat(filepath.Join(root, "magelift.php")); !os.IsNotExist(err) {
				t.Fatal("expected project only in selected app")
			}
		})
	}
}
