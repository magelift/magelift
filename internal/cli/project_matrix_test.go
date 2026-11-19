package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildcontext "github.com/magelift/magelift"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/projectphp"
)

func TestExampleProjectsAcceptBundledComposer(t *testing.T) {
	image, err := buildcontext.ComposerRecipe("")
	if err != nil {
		t.Fatal(err)
	}
	tagged, _, _ := strings.Cut(image, "@")
	version := tagged[strings.LastIndexByte(tagged, ':')+1:]
	for _, example := range []string{"magelift.php", "advanced-aws-fck-nat.php", "sample-shop/magelift.php"} {
		t.Run(example, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "examples", example))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "magelift.php")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			file, err := (&options{configPath: path, planContext: t.Context()}).load()
			if err != nil {
				t.Fatal(err)
			}
			spec, err := file.ResolveBuild()
			if err != nil {
				t.Fatal(err)
			}
			if !composerVersionMatches(spec.Build.Composer.Version, version) {
				t.Fatalf("example requires Composer %q, bundled image supplies %s", spec.Build.Composer.Version, version)
			}
		})
	}
}

func TestInitAppliesCloudAccountRegionAndDomainsAcrossStages(t *testing.T) {
	for _, tc := range []struct{ provider, account, region string }{
		{"aws", "123456780001", "us-east-2"},
		{"gcp", "example-shop-dev", "us-east1"},
		{"azure", "f680e32f-8259-437f-bebd-8acbb89bac33", "northeurope"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			dir := t.TempDir()
			writeInitShopFixture(t, dir, "2.4.9", "open-source")
			path := filepath.Join(dir, "magelift.php")
			var out bytes.Buffer
			cmd := newCommand(&out, &out, nil)
			cmd.SetArgs([]string{"--config", path, "init", "--provider", tc.provider, "--cloud-account", tc.account, "--region", tc.region, "--domain", "store.example.org"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			raw, err := projectphp.Evaluate(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			file, err := config.Load(raw)
			if err != nil {
				t.Fatal(err)
			}
			for name, host := range map[string]string{"production": "store.example.org", "preview": "preview.store.example.org", "staging": "staging.store.example.org", "uat": "uat.store.example.org"} {
				e, err := file.Resolve(name, config.ResolveOptions{})
				if err != nil {
					t.Fatal(err)
				}
				c := e.Config
				account := c.Account
				if tc.provider == "gcp" {
					account = c.Target.GCP.Project
					if c.Target.GCP.Region != tc.region {
						t.Fatal(c.Target.GCP.Region)
					}
				}
				if tc.provider == "azure" {
					account = c.Target.Azure.SubscriptionID
				}
				if c.Domain != host || c.Defaults.Region != tc.region || account != tc.account {
					t.Fatalf("%s %s resolved %q %q %q", tc.provider, name, c.Domain, c.Defaults.Region, account)
				}
			}
		})
	}
}

// Exercise the entire small cross-product at the configuration boundary.
// Provider graph, API emulator and actual Magento execution have separate suites.
func TestStarterEditionVersionCloudAndEnvironmentMatrix(t *testing.T) {
	for _, cloud := range []string{"aws", "gcp", "azure"} {
		for _, edition := range []string{"open-source", "commerce"} {
			for _, release := range []string{"2.4.6-p1", "2.4.8", "2.4.9", "2.4.10"} {
				t.Run(cloud+"/"+edition+"/"+release, func(t *testing.T) {
					doc, err := starterDocument(cloud, inspectedShop{ProjectName: "shop", Edition: edition, Version: release})
					if err != nil {
						t.Fatal(err)
					}
					doc["services"] = map[string]any{"database": map[string]any{"family": "mysql", "version": "8.4"}}
					data, err := json.Marshal(doc)
					if err != nil {
						t.Fatal(err)
					}
					file, err := config.Load(data)
					if err != nil {
						t.Fatal(err)
					}
					for _, env := range []string{"preview", "staging", "uat", "production"} {
						e, err := file.Resolve(env, config.ResolveOptions{})
						if err != nil {
							t.Fatal(env, err)
						}
						if e.Config.Application.Version != release || e.Config.Services["database"].Version != "8.4" || e.Config.Target.Provider != cloud || e.Config.Application.Edition != edition {
							t.Fatalf("lost project selection in %s", env)
						}
						if env == "production" && (!e.Config.Protection || e.Config.Preset != "high-availability") {
							t.Fatal("production lost HA or protection")
						}
					}
				})
			}
		}
	}
}
func TestHeadlessInitAndEnvironmentDiff(t *testing.T) {
	dir := t.TempDir()
	writeInitShopFixture(t, dir, "2.4.9", "open-source")
	path := filepath.Join(dir, "magelift.php")
	var output bytes.Buffer
	cmd := newCommand(&output, &output, nil)
	cmd.SetArgs([]string{"--config", path, "init", "--provider", "aws", "--storefront-origin", "https://www.example.test/"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := projectphp.Evaluate(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"corsOrigins":["https://www.example.test"]`)) {
		t.Fatal("headless CORS origin missing")
	}
	output.Reset()
	cmd = newCommand(&output, &output, nil)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "diff", "staging", "production"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"field": "protection"`) || strings.Contains(output.String(), `"field": "application.mode"`) {
		t.Fatal(output.String())
	}
}
