package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/localdev"
	"github.com/magelift/magelift/internal/projectphp"
)

func TestMigrationProfilesKeepShopServicesAcrossClouds(t *testing.T) {
	for _, provider := range []string{"aws", "gcp", "azure"} {
		for _, source := range []string{"upsun", "compose"} {
			t.Run(provider+"/"+source, func(t *testing.T) {
				root := t.TempDir()
				release := "2.4.8-p5"
				if source == "compose" {
					release = "2.4.9"
				}
				writeInitShopFixture(t, root, release, "community")
				files := map[string]string{"composer.lock": `{"packages":[{"name":"magento/product-community-edition","version":"` + release + `"},{"name":"smile/elasticsuite","version":"2.12.0"}]}`}
				if source == "upsun" {
					files[".platform.app.yaml"] = "name: bookstore\ntype: php:8.3\nrelationships: {db: 'inventory:mysql', cache: 'fast:valkey', sessions: 'ledger:valkey', search: 'index:opensearch', queue: 'events:rabbitmq'}\n"
					files[".platform/services.yaml"] = `inventory: {type: 'mariadb:11.4', disk: 11265, configuration: {properties: {default_charset: utf8mb4, max_allowed_packet: 64M}}}
fast: {type: 'valkey:8.1', configuration: {maxmemory_policy: allkeys-lru}}
ledger: {type: 'valkey-persistent:8.1', disk: 1025, configuration: {maxmemory_policy: noeviction}}
index: {type: 'opensearch:3', configuration: {plugins: [analysis-icu, analysis-phonetic]}}
events: {type: 'rabbitmq:4.1', disk: 2048}
`
					files[".platform.env.yaml"] = "stage: {build: {SCD_THREADS: 2}}"
					files[".magento.env.yaml"] = `stage:
  build:
    SCD_MATRIX:
      Example/books: { language: [de_DE, it_IT] }
      Magento/backend: {language: [en_US]}
    QUALITY_PATCHES: [EXAMPLE-42]
  deploy:
    SESSION_CONFIGURATION: {_merge: true, redis: {max_concurrency: 9, password: synthetic-never-copy}}
    CRON_CONSUMERS_RUNNER: {cron_run: true, max_messages: 43, consumers: [catalog.events]}
  post-deploy:
    WARM_UP_PAGES: [synthetic-never-copy]
`
				} else {
					files["compose.yml"] = `services:
  db: {image: mysql:8.4}
  store: {image: valkey/valkey:9-alpine@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}
  queue: {image: rabbitmq:4-management}
  opensearch: {image: 'registry.example/shop-search:custom'}
`
				}
				for path, body := range files {
					path = filepath.Join(root, path)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
				}
				project := filepath.Join(root, "magelift.php")
				var output bytes.Buffer
				cmd := newCommand(&output, &output, nil)
				cmd.SetArgs([]string{"--config", project, "init", "--from-" + source, "--provider", provider})
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "unmapped") {
					t.Fatalf("expected review file, got %v %s", err, output.String())
				}
				rendered, err := projectphp.Evaluate(t.Context(), project)
				if err != nil {
					t.Fatal(err)
				}
				report, err := os.ReadFile(filepath.Join(root, "magelift.unmapped.md"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(rendered)+string(report), "synthetic-never-copy") {
					t.Fatal("import copied a source value")
				}
				file, err := config.Load(rendered)
				if err != nil {
					t.Fatal(err)
				}
				for _, env := range []string{"staging", "uat", "production", "preview"} {
					e, err := file.Resolve(env, config.ResolveOptions{})
					if err != nil {
						t.Fatalf("%s: %v", env, err)
					}
					c := e.Config
					if err := c.CheckServiceCapabilities(); err != nil {
						t.Fatalf("%s: %v", env, err)
					}
					b := c.Services
					if source == "upsun" && env == "production" {
						warnings := strings.Join(c.ServiceWarnings(), " ")
						role := "services.queue"
						if provider != "aws" {
							role = "services.database"
						}
						if !strings.Contains(warnings, role) {
							t.Fatalf("missing single-replica warning for %s: %s", role, warnings)
						}
					}

					if b["queue"].Family != "rabbitmq" || !slices.Equal(b["search"].Plugins, []string{"analysis-icu", "analysis-phonetic"}) {
						t.Fatal(b)
					}
					if source == "upsun" {
						if b["database"].Family != "mariadb" || b["session"].Version != "8.1" || b["session"].EvictionPolicy != "noeviction" || b["cache"].EvictionPolicy != "allkeys-lru" || b["queue"].Version != "4.1" {
							t.Fatal(b)
						}
						if provider != "aws" && (b["database"].StorageGiB != 12 || b["session"].StorageGiB != 2 || b["queue"].StorageGiB != 2) {
							t.Fatal(b)
						}
						if !strings.Contains(string(report), "stage.post-deploy.WARM_UP_PAGES") || !strings.Contains(string(report), "stage.deploy.SESSION_CONFIGURATION.redis.password") {
							t.Fatal(string(report))
						}
						if b["database"].Parameters["max_allowed_packet"] != "67108864" || b["database"].Parameters["character_set_server"] != "utf8mb4" || !slices.Equal(c.Build.StaticContent.Matrix["Example/books"], []string{"de_DE", "it_IT"}) || !slices.Equal(c.Application.Magento.Consumers.Names, []string{"catalog.events"}) || len(c.Build.StaticContent.Matrix) != 2 || c.Build.StaticContent.Threads != 2 || !slices.Equal(c.Build.QualityPatches, []string{"EXAMPLE-42"}) || c.Application.Magento.Consumers.Mode != "cron" {
							t.Fatal(c.Build)
						}
						if !strings.Contains(c.Application.Magento.Variables["MAGENTO_DC__OVERRIDE"], "max_concurrency") {
							t.Fatal(c.Application.Magento)
						}
					} else if provider != "aws" && !strings.Contains(b["cache"].Image, "@sha256:") {
						t.Fatal("lost source image digest on the self-hosted store")
					} else if b["database"].Family != "mysql" || b["cache"].Version != "9" || b["queue"].Version != "4" {
						t.Fatal(b)
					}
					build, err := file.ResolveBuild()
					if err != nil {
						t.Fatal(err)
					}
					build.Local = c.Local
					plan, err := localdev.Plan(build)
					if err != nil {
						t.Fatal(err)
					}
					compose := localdev.ComposeTemplateFor(plan)
					if !strings.Contains(compose, "opensearch-plugin install") || (source == "upsun" && (!strings.Contains(compose, "noeviction") || !strings.Contains(compose, "allkeys-lru") || plan.Session.Connection.Host != "session")) {
						t.Fatal(compose)
					}
				}
			})
		}
	}
}
