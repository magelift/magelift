package config

import (
	"fmt"
	"strings"
	"testing"
)

const base = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build:
  php: "8.3"
  composer: {credentials: aws-secrets-manager://composer/auth}
  staticContent: {locales: [en_US, fr_FR], strategy: compact}
target: {provider: aws, runtime: ecs-fargate}
defaults: {region: eu-west-3, preset: preview}
environments:
  shared:
    build:
      staticContent: {strategy: standard}
  staging:
    inherits: shared
    account: "123"
    build:
      staticContent: {locales: [de_DE], strategy: null}
extensions:
  vendor.example: {anything: true}
`

func TestResolveMergeAndProvenance(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{
		Builtins:  map[string]any{"defaults": map[string]any{"region": "us-east-1"}},
		Presets:   map[string]map[string]any{"preview": {"build": map[string]any{"staticContent": map[string]any{"locales": []any{"en_US"}}}}},
		Overrides: map[string]any{"defaults": map[string]any{"region": "us-west-2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := effective.Config.Defaults.Region; got != "us-west-2" {
		t.Fatalf("region = %q", got)
	}
	if got := effective.Config.Application.WebRuntime; got != "nginx-fpm" {
		t.Fatalf("web runtime default = %q", got)
	}
	if got := effective.Provenance["application.webRuntime"].Source; got != "built-in defaults" {
		t.Fatalf("web runtime provenance = %q", got)
	}
	locales := effective.Config.Build.StaticContent.Locales
	if len(locales) != 1 || locales[0] != "de_DE" {
		t.Fatalf("lists were not replaced: %#v", locales)
	}
	if effective.Config.Build.StaticContent.Strategy != "" {
		t.Fatal("null did not remove inherited value")
	}
	if got := effective.Provenance["defaults.region"].Source; got != "CLI override" {
		t.Fatalf("provenance = %q", got)
	}
	if !effective.Provenance["build.staticContent.strategy"].Removed {
		t.Fatal("removal provenance missing")
	}
	if !strings.HasPrefix(effective.Fingerprint, "sha256:") {
		t.Fatalf("resolved fingerprint = %q", effective.Fingerprint)
	}
}

func TestLoadRejectsUnregisteredWebRuntimes(t *testing.T) {
	for _, runtime := range []string{"frankenphp-worker", "caddy"} {
		input := strings.Replace(base, "mode: integrated}", "mode: integrated, webRuntime: "+runtime+"}", 1)
		file, err := Load([]byte(input))
		if err != nil {
			t.Fatalf("load webRuntime %q: %v", runtime, err)
		}
		_, err = file.Resolve("staging", ResolveOptions{})
		if err == nil || !strings.Contains(err.Error(), "not registered") {
			t.Fatalf("webRuntime %q error = %v, want missing plugin", runtime, err)
		}
	}
}

func TestLoadFrankenPHPAndApacheRequireAdobeHatch(t *testing.T) {
	for _, runtime := range []string{"frankenphp-classic", "php-apache"} {
		input := strings.Replace(base, "mode: integrated}", "mode: integrated, webRuntime: "+runtime+"}", 1)
		file, err := Load([]byte(input))
		if err != nil {
			t.Fatalf("load webRuntime %q: %v", runtime, err)
		}
		_, err = file.Resolve("staging", ResolveOptions{})
		if err == nil || !strings.Contains(err.Error(), "allowUnsupported") {
			t.Fatalf("webRuntime %q without hatch error = %v", runtime, err)
		}

		hatched := strings.Replace(input, "extensions:\n", "compatibility: {allowUnsupported: true}\nextensions:\n", 1)
		file, err = Load([]byte(hatched))
		if err != nil {
			t.Fatalf("load hatched webRuntime %q: %v", runtime, err)
		}
		effective, err := file.Resolve("staging", ResolveOptions{})
		if err != nil {
			t.Fatalf("hatched webRuntime %q: %v", runtime, err)
		}
		if effective.Compatibility.Status != CompatibilityUnsupportedAllowed {
			t.Fatalf("hatched status = %q", effective.Compatibility.Status)
		}
		found := false
		for _, warning := range effective.Compatibility.Warnings {
			if strings.Contains(warning, "Adobe-unsupported") {
				found = true
			}
		}
		if !found {
			t.Fatalf("hatched webRuntime %q missing Adobe warning: %#v", runtime, effective.Compatibility.Warnings)
		}
	}
}

func TestResolveMagentoRuntimeOverlays(t *testing.T) {
	input := strings.Replace(base, "mode: integrated}", `mode: integrated, magento: {frontName: backend, cookieDomain: ".shop.test", consumers: {mode: processes, names: [product_action_attribute.update]}}}`, 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := effective.Config.Application.Magento
	if got.FrontName != "backend" || got.CookieDomain != ".shop.test" || got.Consumers.Mode != "processes" {
		t.Fatalf("magento runtime = %#v", got)
	}
	if effective.Provenance["application.magento.frontName"].Source == "" {
		t.Fatalf("frontName provenance = %#v", effective.Provenance["application.magento.frontName"])
	}
}

func TestResolveRejectsDuplicateQualityPatches(t *testing.T) {
	input := strings.Replace(base, `php: "8.3"`, `php: "8.3"
  qualityPatches: [ACSD-123, ACSD-123]`, 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate qualityPatches error = %v", err)
	}
}

func TestResolveRejectsPlaintextMagentoSecretOverlay(t *testing.T) {
	input := strings.Replace(base, "mode: integrated}", `mode: integrated, magento: {variables: {MAGENTO_DC_CRYPT__KEY: "not-a-secret-ref"}}}`, 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "secret reference") {
		t.Fatalf("plaintext crypt overlay error = %v", err)
	}
}

func TestResolveResilienceProjectionTarget(t *testing.T) {
	input := base + `
resilience:
  profileId: preview
  availabilityTarget: "99.9"
  rpoSeconds: 60
  rtoSeconds: 900
  retentionDays: 7
  recoveryScope: application
  failoverOwner: operator
  fencingPolicy: single-writer
  recoveryDestinations: [same-region-isolated]
  projection:
    runtime: ecs
    cluster: shop
    service: web
    container: php
  dataClasses:
    - name: cache
      sourceOfTruth: database
      backupMethod: reconstruct
      restoreMethod: cache-warmup
      integrityMethod: application-read
      lossSemantics: rebuildable
      retentionDays: 1
      ownershipMarker: magelift/test/cache
`
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	projection := effective.Config.Resilience.Projection
	if projection == nil || projection.Runtime != "ecs" || projection.Cluster != "shop" || projection.Service != "web" || projection.Task != "" || projection.Container != "php" {
		t.Fatalf("projection target = %#v", projection)
	}
	if got := effective.Provenance["resilience.projection.service"].Source; got != "project" {
		t.Fatalf("projection provenance = %q", got)
	}

	kubernetesInput := strings.Replace(input, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: gcp
  runtime: gke-autopilot
  gcp: {project: example-project}
`, 1)
	kubernetesInput = strings.Replace(kubernetesInput, "aws-secrets-manager://composer/auth", "gcp-secret-manager://composer/auth", 1)
	kubernetesInput = strings.Replace(kubernetesInput, "    runtime: ecs\n    cluster: shop\n    service: web\n    container: php", "    runtime: kubernetes\n    namespace: magento\n    workload: web\n    container: php", 1)
	kubernetesFile, err := Load([]byte(kubernetesInput))
	if err != nil {
		t.Fatal(err)
	}
	kubernetesEffective, err := kubernetesFile.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if projection := kubernetesEffective.Config.Resilience.Projection; projection == nil || projection.Runtime != "kubernetes" || projection.Namespace != "magento" || projection.Workload != "web" {
		t.Fatalf("Kubernetes projection target = %#v", projection)
	}
}

func TestResolveRejectsInvalidResilienceProjectionTarget(t *testing.T) {
	input := base + `
resilience:
  profileId: preview
  availabilityTarget: "99.9"
  rpoSeconds: 60
  rtoSeconds: 900
  retentionDays: 7
  recoveryScope: application
  failoverOwner: operator
  fencingPolicy: single-writer
  recoveryDestinations: [same-region-isolated]
  projection: {runtime: ecs, cluster: shop, service: web, task: task-1, container: php}
  dataClasses:
    - {name: cache, sourceOfTruth: database, backupMethod: reconstruct, restoreMethod: cache-warmup, integrityMethod: application-read, lossSemantics: rebuildable, retentionDays: 1, ownershipMarker: magelift/test/cache}
`
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "exactly one of service or task") {
		t.Fatalf("invalid projection target was accepted: %v", err)
	}
}

func TestResolveManagedServiceProtectionSettings(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: digital-lab-341608
    cloudSqlAvailability: REGIONAL
    cloudSqlBackupEnabled: true
    cloudSqlBinaryLogEnabled: true
    cloudSqlBackupRetentionCount: 14
    cloudSqlTransactionLogRetentionDays: 7
    cloudSqlBackupStartTime: "03:30"
    cloudSqlBackupLocation: europe-west1
    cloudSqlDeletionProtection: true
    memorystoreDeletionProtection: true
    memorystorePscConnectionLimit: 4
defaults: {region: europe-west1, preset: standard}
environments: {staging: {}}
`
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gcp := effective.Config.Target.GCP
	if gcp == nil || gcp.CloudSQLBackupEnabled == nil || !*gcp.CloudSQLBackupEnabled || gcp.CloudSQLBackupRetentionCount == nil || *gcp.CloudSQLBackupRetentionCount != 14 || gcp.CloudSQLTransactionLogRetention == nil || *gcp.CloudSQLTransactionLogRetention != 7 || gcp.MemorystorePSCConnectionLimit == nil || *gcp.MemorystorePSCConnectionLimit != 4 {
		t.Fatalf("managed service protection settings were not decoded: %#v", gcp)
	}
	if gcp.CloudSQLBackupStartTime != "03:30" || gcp.CloudSQLBackupLocation != "europe-west1" || gcp.CloudSQLDeletionProtection == nil || !*gcp.CloudSQLDeletionProtection || gcp.MemorystoreDeletionProtection == nil || !*gcp.MemorystoreDeletionProtection {
		t.Fatalf("managed service protection values were not decoded: %#v", gcp)
	}
	for _, path := range []string{"target.gcp.cloudSqlBackupEnabled", "target.gcp.cloudSqlBackupRetentionCount", "target.gcp.cloudSqlDeletionProtection", "target.gcp.memorystorePscConnectionLimit"} {
		if got := effective.Provenance[path].Source; got != "project" {
			t.Fatalf("%s provenance = %q, want project", path, got)
		}
	}
}

func TestResolveRejectsDisabledRegionalCloudSQLBackups(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target:
  provider: gcp
  runtime: gke-autopilot
  gcp: {project: digital-lab-341608, cloudSqlAvailability: REGIONAL, cloudSqlBackupEnabled: false}
defaults: {region: europe-west1, preset: standard}
environments: {staging: {}}
`
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "cloudSqlBackupEnabled cannot be false") {
		t.Fatalf("disabled regional Cloud SQL backups were accepted: %v", err)
	}
}

func TestResolveMaterializesNamedGCPBackupDefaults(t *testing.T) {
	input := strings.Replace(base, "aws-secrets-manager://composer/auth", "gcp-secret-manager://composer/auth", 1)
	input = strings.Replace(input, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: gcp
  runtime: gke-autopilot
  gcp: {project: example-gcp-project, region: europe-west1}
`, 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: europe-west1, preset: standard}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gcp := effective.Config.Target.GCP
	if gcp == nil || gcp.CloudSQLBackupEnabled == nil || !*gcp.CloudSQLBackupEnabled || gcp.CloudSQLBinaryLogEnabled == nil || !*gcp.CloudSQLBinaryLogEnabled || gcp.CloudSQLBackupRetentionCount == nil || *gcp.CloudSQLBackupRetentionCount != 8 || gcp.CloudSQLTransactionLogRetention == nil || *gcp.CloudSQLTransactionLogRetention != 7 || gcp.CloudSQLTier != "db-perf-optimized-N-2" || gcp.MemorystoreEngineVersion != "VALKEY_8_0" || gcp.EnableCloudArmor == nil || !*gcp.EnableCloudArmor {
		t.Fatalf("GCP backup defaults = %#v", gcp)
	}
	for _, path := range []string{
		"target.gcp.cloudSqlBackupEnabled",
		"target.gcp.cloudSqlBinaryLogEnabled",
		"target.gcp.cloudSqlBackupRetentionCount",
		"target.gcp.cloudSqlTransactionLogRetentionDays",
		"target.gcp.enableCloudArmor",
	} {
		if got := effective.Provenance[path].Source; got != "preset standard" {
			t.Fatalf("%s provenance = %q, want preset standard", path, got)
		}
	}

	t.Run("Magento 2.4.9 selects Valkey 9.0", func(t *testing.T) {
		updated := strings.Replace(input, "2.4.8-p5", "2.4.9", 1)
		updated = strings.Replace(updated, `php: "8.3"`, `php: "8.5"`, 1)
		file, err := Load([]byte(updated))
		if err != nil {
			t.Fatal(err)
		}
		effective, err := file.Resolve("staging", ResolveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := effective.Config.Target.GCP.MemorystoreEngineVersion; got != "VALKEY_9_0" {
			t.Fatalf("GCP Memorystore default engine = %q, want VALKEY_9_0", got)
		}
	})
}

func TestResolveRejectsCloudSQLRetentionThatCannotCoverTransactionLogs(t *testing.T) {
	input := strings.Replace(base, "aws-secrets-manager://composer/auth", "gcp-secret-manager://composer/auth", 1)
	input = strings.Replace(input, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    cloudSqlAvailability: ZONAL
    cloudSqlBackupRetentionCount: 3
    cloudSqlTransactionLogRetentionDays: 7
`, 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: europe-west1, preset: preview}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "cannot exceed cloudSqlBackupRetentionCount") {
		t.Fatalf("invalid Cloud SQL retention relationship was accepted: %v", err)
	}
}

func TestFastlyEdgeRequiresSecretReference(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "edge: {externalProvider: fastly, serviceId: \"svc-123\", tokenSecret: \"not-a-token\"}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "edge.tokenSecret") {
		t.Fatalf("plaintext Fastly token was accepted: %v", err)
	}
}

func TestFastlyEdgeIntentResolvesWithSecretReference(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "edge: {externalProvider: fastly, serviceId: \"svc-123\", tokenSecret: \"aws-secrets-manager://magelift/fastly-token\", domains: [shop.example.com], originHealthRef: health/magento, health: {expectedCname: dualstack.e.sni.global.fastly.net}, purgeOnDeploy: true}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Edge.ExternalProvider != "fastly" || effective.Config.Edge.TokenSecret == "" || !effective.Config.Edge.PurgeOnDeploy {
		t.Fatalf("edge intent = %#v", effective.Config.Edge)
	}
}

func TestObservabilityIntentResolvesWithoutVendorCredentials(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "observability:\n  externalProvider: datadog\n  endpoint: https://otlp.example.com\n  serviceName: magento\n  environment: staging\n  logs: true\n  metrics: true\n  labels: {team: commerce}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	observability := effective.Config.Observability
	if observability.ExternalProvider != "datadog" || !observability.Logs || !observability.Metrics || observability.Labels["team"] != "commerce" {
		t.Fatalf("observability intent = %#v", observability)
	}
}

func TestObservabilityCredentialReferencesRejectPlaintext(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "observability:\n  externalProvider: datadog\n  credentialReferences: [raw-token]\n  logs: true\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "credentialReferences") {
		t.Fatalf("plaintext observability credential was accepted: %v", err)
	}
}

func TestObservabilityIntentRequiresSignals(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "observability: {externalProvider: datadog}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "observability must enable") {
		t.Fatalf("signal-free observability intent was accepted: %v", err)
	}
}

func TestObservabilityNativeReferenceRequiresNativeDestination(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "observability: {nativeReference: stream-123, logs: true}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "nativeProvider") {
		t.Fatalf("native reference without a native provider was accepted: %v", err)
	}
}

func TestObservabilityNativeReferenceResolvesAsOpaqueIdentity(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "observability: {nativeProvider: ovh-logs-data-platform, nativeReference: stream-123, signals: [audit-events]}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Observability.NativeReference != "stream-123" || effective.Config.Observability.NativeProvider != "ovh-logs-data-platform" {
		t.Fatalf("opaque native destination identity = %#v", effective.Config.Observability)
	}
}

func TestRemovedSingularProviderFieldsAreRejected(t *testing.T) {
	for name, fragment := range map[string]string{
		"edge":          "edge: {provider: fastly}",
		"observability": "observability: {provider: newrelic}",
	} {
		t.Run(name, func(t *testing.T) {
			input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", fragment+"\ndefaults: {region: eu-west-3, preset: preview}", 1)
			if _, err := Load([]byte(input)); err == nil || !strings.Contains(err.Error(), "field provider") {
				t.Fatalf("removed %s provider field was accepted: %v", name, err)
			}
		})
	}
}

func TestObservabilityAlertsAndSLOsResolveWithSecretSafeMetadata(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", `observability:
  nativeProvider: cloudwatch
  signals: [backup-age, application-health]
  dataResidency: eu
  alerts:
    - {id: backup-stale, signal: backup-age, severity: critical, operator: gt, threshold: 300, windowSeconds: 300, owner: oncall, runbookUrl: https://runbooks.example/backup, deduplicationKey: shop/backup}
  dashboards:
    - {id: resilience, signals: [backup-age, recovery-state], owner: oncall}
  slos:
    - {id: availability, signal: application-health, target: 0.999, windowSeconds: 2592000, owner: oncall, runbookUrl: https://runbooks.example/availability, errorBudgetPolicy: page-on-burn}
defaults: {region: eu-west-3, preset: preview}`, 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Config.Observability.Alerts) != 1 || len(effective.Config.Observability.Dashboards) != 1 || len(effective.Config.Observability.SLOs) != 1 || effective.Config.Observability.DataResidency != "eu" {
		t.Fatalf("observability policy = %#v", effective.Config.Observability)
	}
}

func TestResolveAppliesBoundedDefaultsWhenAWSTargetIsConfigured(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.Catalog.Versions.OpenSearch != "OpenSearch_3.1" || aws.Catalog.Versions.Valkey != "8.1" {
		t.Fatalf("compatibility defaults = %#v", aws)
	}
	if aws.Catalog.Fargate.DesiredCount != 1 || !aws.Catalog.Search.AcceptColdStarts {
		t.Fatalf("preview defaults = %#v", aws.Catalog)
	}
	if aws.Catalog.CacheSnapshotRetentionLimit == nil || *aws.Catalog.CacheSnapshotRetentionLimit != 0 {
		t.Fatalf("preview cache snapshot default = %#v", aws.Catalog.CacheSnapshotRetentionLimit)
	}
	if aws.Catalog.Fargate.ComputeMode != "fargate" || aws.Catalog.Fargate.CPU != 512 || aws.Catalog.Fargate.MemoryMiB != 1024 || aws.Catalog.Aurora.InstanceClass != "db.t4g.micro" || aws.Catalog.Aurora.MaximumACU != 4 || aws.Catalog.Valkey.NodeType != "cache.t4g.micro" || aws.Catalog.Search.MaximumIndexingOCU != 2 || aws.Catalog.Search.MaximumSearchOCU != 2 {
		t.Fatalf("preview capacity defaults = %#v", aws.Catalog)
	}
	if aws.VPCCIDR != "10.42.0.0/16" || strings.Join(aws.AvailabilityZones, ",") != "eu-west-3a,eu-west-3b" {
		t.Fatalf("preview network defaults = %#v", aws)
	}
	if aws.Catalog.Retention.LogDays != 7 || aws.Catalog.Retention.BackupDays != 1 || aws.Catalog.Retention.ArtifactDays != 7 {
		t.Fatalf("preview retention defaults = %#v", aws.Catalog.Retention)
	}
	if got := effective.Provenance["target.aws.catalog.versions.openSearch"].Source; got != "compatibility defaults" {
		t.Fatalf("compatibility provenance = %q", got)
	}
	if got := effective.Provenance["target.aws.catalog.fargate.desiredCount"].Source; got != "preset preview" {
		t.Fatalf("preset provenance = %q", got)
	}
	for _, path := range []string{"target.aws.vpcCidr", "target.aws.availabilityZones"} {
		if got := effective.Provenance[path].Source; got != "AWS network defaults" {
			t.Fatalf("network provenance for %s = %q", path, got)
		}
	}
	if got := effective.Provenance["target.aws.catalog.databaseDeletionProtection"].Source; got != "environment class defaults" {
		t.Fatalf("AWS deletion-safety provenance = %q", got)
	}
}

func TestResolveDoesNotInventOwnedNetworkDefaultsForAdoptedAWSNetwork(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: aws
  runtime: ecs-fargate
  aws:
    existing:
      network: {provider: aws, kind: network, externalId: vpc-existing}
`, 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.VPCCIDR != "" || len(aws.AvailabilityZones) != 0 {
		t.Fatalf("adopted network unexpectedly received owned-network defaults: %#v", aws)
	}
}

func TestResolveMaterializesContextualProductionDefaults(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: gcp
  runtime: gke-autopilot
  gcp: {project: example-gcp-project}
`, 1)
	input = strings.Replace(input, "aws-secrets-manager://composer/auth", "gcp-secret-manager://composer/auth", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: europe-west1, preset: preview}", 1)
	input = strings.Replace(input, "    account: \"123\"\n", "    account: \"123\"\n    class: production\n", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gcp := effective.Config.Target.GCP
	if gcp == nil || gcp.EnableCloudArmor == nil || !*gcp.EnableCloudArmor {
		t.Fatalf("production preview Cloud Armor default = %#v", gcp)
	}
	if got := effective.Provenance["target.gcp.enableCloudArmor"].Source; got != "environment class defaults" {
		t.Fatalf("production preview Cloud Armor provenance = %q", got)
	}
}

func TestResolveMaterializesNamedAWSStandardDurabilityDefaults(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {}}", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: eu-west-3, preset: standard}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := effective.Config.Target.AWS.Catalog
	if catalog.CacheSnapshotRetentionLimit == nil || *catalog.CacheSnapshotRetentionLimit != 7 {
		t.Fatalf("standard cache snapshot default = %#v", catalog.CacheSnapshotRetentionLimit)
	}
	if catalog.DatabaseDeletionProtection == nil || *catalog.DatabaseDeletionProtection || catalog.DatabaseDeleteAutomatedBackups == nil || !*catalog.DatabaseDeleteAutomatedBackups {
		t.Fatalf("standard cleanup defaults = %#v", catalog)
	}
	if catalog.QueueMode != "ecs-rabbitmq" {
		t.Fatalf("standard queueMode default = %q, want ecs-rabbitmq", catalog.QueueMode)
	}
	if got := effective.Provenance["target.aws.catalog.queueMode"].Source; got != "preset standard" {
		t.Fatalf("queueMode provenance = %q, want preset standard", got)
	}
	for _, path := range []string{"target.aws.catalog.cacheSnapshotRetentionLimit", "target.aws.catalog.databaseDeletionProtection", "target.aws.catalog.databaseDeleteAutomatedBackups"} {
		if got := effective.Provenance[path].Source; got == "" {
			t.Fatalf("%s has no provenance", path)
		}
	}
}

func TestResolveDefaultsAWSQueueModeByPreset(t *testing.T) {
	tests := []struct {
		preset string
		want   string
		source string
	}{
		{preset: "preview", want: "db", source: "preset preview"},
		{preset: "standard", want: "ecs-rabbitmq", source: "preset standard"},
		{preset: "high-availability", want: "ecs-rabbitmq", source: "preset high-availability"},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {}}", 1)
			input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: eu-west-3, preset: "+tt.preset+"}", 1)
			file, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			effective, err := file.Resolve("staging", ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := effective.Config.Target.AWS.Catalog.QueueMode; got != tt.want {
				t.Fatalf("queueMode = %q, want %q", got, tt.want)
			}
			if got := effective.Provenance["target.aws.catalog.queueMode"].Source; got != tt.source {
				t.Fatalf("queueMode provenance = %q, want %q", got, tt.source)
			}
			joined := strings.Join(effective.Compatibility.Warnings, "\n")
			if strings.Contains(joined, "Amazon MQ") {
				t.Fatalf("unset %s must not experimental-warn Amazon MQ: %q", tt.preset, joined)
			}
		})
	}
}

func TestResolveMaterializesNamedAWSEKSDefaults(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: eks, aws: {}}", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: eu-west-3, preset: standard}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.Catalog.EKS.ComputeMode != "auto-mode" || aws.Catalog.EKS.KubernetesVersion != "1.36" || aws.Catalog.EKS.CPURequest != "500m" || aws.Catalog.EKS.MemoryRequest != "1Gi" || aws.Catalog.EKS.DesiredWebReplicas != 2 || aws.Catalog.Aurora.InstanceClass != "db.r7g.large" || aws.Catalog.Aurora.InstanceCount != 2 || aws.Catalog.Valkey.NodeType != "cache.r7g.large" || aws.Catalog.Valkey.ReplicaCount != 1 {
		t.Fatalf("EKS capacity defaults = %#v", aws.Catalog)
	}
	for _, path := range []string{"target.aws.catalog.eks.computeMode", "target.aws.catalog.eks.kubernetesVersion", "target.aws.catalog.eks.cpuRequest", "target.aws.catalog.aurora.instanceClass", "target.aws.catalog.valkey.nodeType"} {
		if got := effective.Provenance[path].Source; got != "preset standard" {
			t.Fatalf("%s provenance = %q, want preset standard", path, got)
		}
	}
}

func TestResolveMaterializesNamedAWSHighAvailabilityDefaults(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: eks, aws: {}}", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: eu-west-3, preset: high-availability}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.Catalog.EKS.DesiredWebReplicas != 3 || aws.Catalog.Aurora.InstanceCount != 3 || aws.Catalog.Valkey.ReplicaCount != 2 || aws.Catalog.Search.InstanceCount != 3 || aws.Catalog.Search.DedicatedMasterType != "r6g.large.search" || aws.Catalog.Search.DedicatedMasterCount != 3 || aws.Catalog.Search.EBSVolumeSizeGiB != 400 {
		t.Fatalf("AWS high-availability capacity defaults = %#v", aws.Catalog)
	}
	for _, path := range []string{"target.aws.catalog.eks.desiredWebReplicas", "target.aws.catalog.aurora.instanceCount", "target.aws.catalog.valkey.replicaCount", "target.aws.catalog.search.instanceCount", "target.aws.catalog.search.dedicatedMasterType", "target.aws.catalog.search.dedicatedMasterCount"} {
		if got := effective.Provenance[path].Source; got != "preset high-availability" {
			t.Fatalf("%s provenance = %q, want preset high-availability", path, got)
		}
	}
}

func TestResolveOmitsManagedNATDefaultsForExistingAWSNetwork(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: aws
  runtime: ecs-fargate
  aws:
    existing:
      network: {provider: aws, kind: network, externalId: vpc-0123456789abcdef0}
      publicSubnetIds: [subnet-0123456789abcdef0, subnet-0123456789abcdef1]
      privateSubnetIds: [subnet-0123456789abcdef2, subnet-0123456789abcdef3]
      dataSubnetIds: [subnet-0123456789abcdef4, subnet-0123456789abcdef5]
`, 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.NatMode != "" || aws.NatTopology != "" || aws.NatReplacementMode != "" || aws.NatInstanceType != "" {
		t.Fatalf("managed NAT defaults leaked into an adopted network: %#v", aws)
	}
	for _, path := range []string{"target.aws.natMode", "target.aws.natTopology"} {
		if !effective.Provenance[path].Removed {
			t.Fatalf("%s was not marked removed in provenance: %#v", path, effective.Provenance[path])
		}
	}
}

func TestResolveMapsAWSManagedDurabilitySettings(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", `target:
  provider: aws
  runtime: ecs-fargate
  aws:
    catalog:
      databaseBackupWindow: "03:00-04:00"
      databaseMaintenanceWindow: "sun:05:00-sun:06:00"
      databaseDeletionProtection: false
      databaseDeleteAutomatedBackups: true
      cacheSnapshotRetentionLimit: 0
`, 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := effective.Config.Target.AWS.Catalog
	if catalog.DatabaseBackupWindow != "03:00-04:00" || catalog.DatabaseMaintenanceWindow != "sun:05:00-sun:06:00" {
		t.Fatalf("AWS managed-service windows = %#v", catalog)
	}
	if catalog.DatabaseDeletionProtection == nil || *catalog.DatabaseDeletionProtection {
		t.Fatalf("database deletion protection = %#v", catalog.DatabaseDeletionProtection)
	}
	if catalog.DatabaseDeleteAutomatedBackups == nil || !*catalog.DatabaseDeleteAutomatedBackups {
		t.Fatalf("database automated-backup deletion = %#v", catalog.DatabaseDeleteAutomatedBackups)
	}
	if catalog.CacheSnapshotRetentionLimit == nil || *catalog.CacheSnapshotRetentionLimit != 0 {
		t.Fatalf("cache snapshot retention = %#v", catalog.CacheSnapshotRetentionLimit)
	}
	for _, path := range []string{
		"target.aws.catalog.databaseBackupWindow",
		"target.aws.catalog.databaseMaintenanceWindow",
		"target.aws.catalog.databaseDeletionProtection",
		"target.aws.catalog.databaseDeleteAutomatedBackups",
		"target.aws.catalog.cacheSnapshotRetentionLimit",
	} {
		if got := effective.Provenance[path].Source; got != "project" {
			t.Fatalf("%s provenance = %q", path, got)
		}
	}
}

func TestResolveRejectsInvalidAWSManagedDurabilitySettings(t *testing.T) {
	inputs := []string{
		"databaseBackupWindow: 25:00-04:00",
		"databaseMaintenanceWindow: monday:05:00-sun:06:00",
		"cacheSnapshotWindow: 03:00-04:00",
		"cacheSnapshotRetentionLimit: 36",
	}
	for _, setting := range inputs {
		t.Run(setting, func(t *testing.T) {
			input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {"+setting+"}}}", 1)
			f, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Resolve("staging", ResolveOptions{}); err == nil {
				t.Fatalf("invalid AWS durability setting %q was accepted", setting)
			}
		})
	}
}

func TestResolveMapsAdvancedGCPSettingsAndPreservesProvenance(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build:
  php: "8.5"
  composer: {credentials: gcp-secret-manager://composer/auth}
target:
  provider: gcp
  runtime: gke-standard
  gcp:
    project: example-gcp-project
    region: europe-west1
    zones: [europe-west1-b, europe-west1-c, europe-west1-d]
    networkCidr: 10.60.0.0/16
    cloudSqlTier: db-c4a-highmem-2
    cloudSqlAvailability: ZONAL
    memorystoreNodeType: STANDARD_SMALL
    memorystoreShardCount: 4
    memorystoreReplicas: 3
    memorystoreEngineVersion: VALKEY_9_1
    memorystoreMode: CLUSTER
    memorystoreZoneDistributionMode: SINGLE_ZONE
    memorystoreZone: europe-west1-b
    openSearchMode: disabled
    openSearchReplicas: 0
    openSearchImage: registry.example/opensearch@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    queueMode: rabbitmq
    queueReplicas: 4
    rabbitMqImage: registry.example/rabbitmq@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    releaseChannel: STABLE
    kubernetesVersion: "1.32"
    clusterIpv4Cidr: 10.64.0.0/16
    servicesIpv4Cidr: 10.65.0.0/20
    standardNodeType: n2-standard-8
    standardNodeCount: 4
    standardNodeMinCount: 3
    standardNodeMaxCount: 6
    standardNodeDiskType: pd-ssd
    standardNodeDiskSizeGiB: 200
    standardNodeImageType: UBUNTU_CONTAINERD
    standardNodeSpot: true
    autopilotCpuRequest: "1"
    autopilotMemoryRequest: 2Gi
    desiredWebReplicas: 5
    queueConsumerCount: 6
    enableCloudArmor: false
defaults: {region: europe-west1, preset: standard}
environments: {staging: {}}
`
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gcp := effective.Config.Target.GCP
	if gcp == nil {
		t.Fatal("resolved GCP target is nil")
	}
	if gcp.MemorystoreReplicas == nil || *gcp.MemorystoreReplicas != 3 || gcp.MemorystoreShardCount == nil || *gcp.MemorystoreShardCount != 4 || gcp.CloudSQLTier != "db-c4a-highmem-2" || gcp.CloudSQLAvailability != "ZONAL" || gcp.MemorystoreEngineVersion != "VALKEY_9_1" || gcp.MemorystoreMode != "CLUSTER" || gcp.MemorystoreZoneDistributionMode != "SINGLE_ZONE" || gcp.MemorystoreZone != "europe-west1-b" {
		t.Fatalf("managed service settings were not resolved: %#v", gcp)
	}
	if gcp.OpenSearchMode != "disabled" || gcp.QueueMode != "rabbitmq" || gcp.QueueReplicas != 4 || gcp.QueueConsumerCount != 6 {
		t.Fatalf("workload modes were not resolved: %#v", gcp)
	}
	if gcp.ReleaseChannel != "STABLE" || gcp.KubernetesVersion != "1.32" || gcp.StandardNodeType != "n2-standard-8" || gcp.StandardNodeCount != 4 || !gcp.StandardNodeSpot {
		t.Fatalf("GKE Standard settings were not resolved: %#v", gcp)
	}
	if gcp.DesiredWebReplicas != 5 || gcp.EnableCloudArmor == nil || *gcp.EnableCloudArmor {
		t.Fatalf("application and edge settings were not resolved: %#v", gcp)
	}
	for _, path := range []string{
		"target.gcp.cloudSqlTier",
		"target.gcp.cloudSqlAvailability",
		"target.gcp.memorystoreShardCount",
		"target.gcp.memorystoreReplicas",
		"target.gcp.memorystoreMode",
		"target.gcp.memorystoreZoneDistributionMode",
		"target.gcp.memorystoreZone",
		"target.gcp.openSearchMode",
		"target.gcp.queueMode",
		"target.gcp.releaseChannel",
		"target.gcp.kubernetesVersion",
		"target.gcp.standardNodeType",
		"target.gcp.standardNodeSpot",
		"target.gcp.enableCloudArmor",
	} {
		if got := effective.Provenance[path].Source; got != "project" {
			t.Fatalf("%s provenance = %q, want project", path, got)
		}
	}
}

func TestResolveMapsScalewayAndOVHManagedServiceDurabilitySettings(t *testing.T) {
	t.Run("scaleway preserves explicit disabled backups and encryption", func(t *testing.T) {
		input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target:
  provider: scaleway
  runtime: kapsule
  scaleway:
    projectId: 11111111-1111-1111-1111-111111111111
    databaseBackupEnabled: false
    databaseEncryptionAtRest: true
defaults: {region: fr-par, preset: standard}
environments: {staging: {}}
`
		file, err := Load([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		effective, err := file.Resolve("staging", ResolveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		scaleway := effective.Config.Target.Scaleway
		if scaleway == nil || scaleway.DatabaseBackupEnabled == nil || *scaleway.DatabaseBackupEnabled || scaleway.DatabaseEncryptionAtRest == nil || !*scaleway.DatabaseEncryptionAtRest {
			t.Fatalf("Scaleway durability settings were not resolved: %#v", scaleway)
		}
		for _, path := range []string{"target.scaleway.databaseBackupEnabled", "target.scaleway.databaseEncryptionAtRest"} {
			if got := effective.Provenance[path].Source; got != "project" {
				t.Fatalf("%s provenance = %q, want project", path, got)
			}
		}
	})

	t.Run("ovh preserves backup destinations and deletion protection", func(t *testing.T) {
		input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    region: GRA9
    databaseBackupTime: "03:30"
    databaseBackupRegions: [GRA9, GRA11]
    databaseDeletionProtection: true
    valkeyBackupTime: "04:00"
    valkeyBackupRegions: [GRA11]
    valkeyDeletionProtection: true
defaults: {region: GRA9, preset: standard}
environments: {staging: {}}
`
		file, err := Load([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		effective, err := file.Resolve("staging", ResolveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ovh := effective.Config.Target.OVH
		if ovh == nil || ovh.DatabaseBackupTime != "03:30" || len(ovh.DatabaseBackupRegions) != 2 || ovh.DatabaseDeletionProtection == nil || !*ovh.DatabaseDeletionProtection || ovh.ValkeyBackupTime != "04:00" || len(ovh.ValkeyBackupRegions) != 1 || ovh.ValkeyDeletionProtection == nil || !*ovh.ValkeyDeletionProtection {
			t.Fatalf("OVH durability settings were not resolved: %#v", ovh)
		}
		for _, path := range []string{"target.ovh.databaseBackupTime", "target.ovh.databaseBackupRegions", "target.ovh.databaseDeletionProtection", "target.ovh.valkeyBackupTime", "target.ovh.valkeyBackupRegions", "target.ovh.valkeyDeletionProtection"} {
			if got := effective.Provenance[path].Source; got != "project" {
				t.Fatalf("%s provenance = %q, want project", path, got)
			}
		}
	})
}

func TestResolveMaterializesNamedDefaultsForScalewayAndOVH(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, cfg Config)
		paths []string
	}{
		{
			name: "scaleway",
			input: `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: scaleway
  runtime: kapsule
  scaleway: {projectId: 11111111-1111-1111-1111-111111111111, region: fr-par}
defaults: {region: fr-par, preset: standard}
environments: {staging: {}}
`,
			check: func(t *testing.T, cfg Config) {
				scw := cfg.Target.Scaleway
				if scw == nil || scw.NetworkCIDR != "172.16.0.0/22" || scw.DatabaseNodeType != "DB-GP-S" || scw.RedisNodeType != "RED1-S" || scw.KapsuleVersion != "1.36.1" || scw.NodeCount != 2 || scw.DesiredWebReplicas != 2 || scw.QueueConsumerCount != 1 || scw.DatabaseBackupEnabled == nil || !*scw.DatabaseBackupEnabled || scw.DatabaseBackupFrequency == nil || *scw.DatabaseBackupFrequency != 24 || scw.DatabaseBackupRetention == nil || *scw.DatabaseBackupRetention != 7 || scw.DatabaseBackupSameRegion == nil || !*scw.DatabaseBackupSameRegion || scw.DatabaseEncryptionAtRest == nil || !*scw.DatabaseEncryptionAtRest {
					t.Fatalf("Scaleway defaults = %#v", scw)
				}
			},
			paths: []string{"target.scaleway.networkCidr", "target.scaleway.databaseNodeType", "target.scaleway.kapsuleVersion", "target.scaleway.nodeCount", "target.scaleway.queueConsumerCount", "target.scaleway.databaseBackupEnabled", "target.scaleway.databaseBackupFrequencyHours", "target.scaleway.databaseBackupRetentionDays", "target.scaleway.databaseBackupSameRegion", "target.scaleway.databaseEncryptionAtRest"},
		},
		{
			name: "ovh",
			input: `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh: {serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx, region: GRA9}
defaults: {region: GRA9, preset: standard}
environments: {staging: {}}
`,
			check: func(t *testing.T, cfg Config) {
				ovh := cfg.Target.OVH
				if ovh == nil || ovh.NetworkCIDR != "10.30.0.0/16" || ovh.DatabaseFlavor != "b3-8" || ovh.DatabasePlan != "production" || ovh.DatabaseNodeCount != 2 || ovh.ValkeyFlavor != "b3-8" || ovh.ValkeyPlan != "production" || ovh.ValkeyNodeCount != 2 || ovh.ValkeyVersion != "8.1" || ovh.MKSPlan != "standard" || ovh.NodeCount != 2 || ovh.DesiredWebReplicas != 2 || ovh.QueueConsumerCount != 1 {
					t.Fatalf("OVH defaults = %#v", ovh)
				}
			},
			paths: []string{"target.ovh.networkCidr", "target.ovh.databaseFlavor", "target.ovh.databasePlan", "target.ovh.databaseNodeCount", "target.ovh.valkeyFlavor", "target.ovh.valkeyPlan", "target.ovh.valkeyNodeCount", "target.ovh.valkeyVersion", "target.ovh.mksPlan", "target.ovh.nodeCount", "target.ovh.queueConsumerCount"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, err := Load([]byte(test.input))
			if err != nil {
				t.Fatal(err)
			}
			effective, err := f.Resolve("staging", ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, effective.Config)
			for _, path := range test.paths {
				if got := effective.Provenance[path].Source; got != "preset standard" {
					t.Fatalf("%s provenance = %q, want preset standard", path, got)
				}
			}
		})
	}
}

func TestResolveMaterializesLowCostOVHPreviewManagedServiceDefaults(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh: {serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx, region: EU-WEST-PAR}
defaults: {region: EU-WEST-PAR, preset: preview}
environments: {preview: {class: preview}}
`
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("preview", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ovh := effective.Config.Target.OVH
	if ovh == nil || ovh.DatabasePlan != "discovery" || ovh.DatabaseNodeCount != 1 || ovh.ValkeyPlan != "discovery" || ovh.ValkeyNodeCount != 1 {
		t.Fatalf("OVH preview defaults = %#v", ovh)
	}
}

func TestResolveRejectsUnsupportedOVHManagedServiceSettings(t *testing.T) {
	const ovhYAML = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    region: EU-WEST-PAR
    zones: [eu-west-par-a, eu-west-par-b]
    databasePlan: business
    databaseVersion: 8.4
    databaseNodeCount: 2
    valkeyPlan: business
    valkeyVersion: 8.1
    valkeyNodeCount: 2
defaults: {region: EU-WEST-PAR, preset: standard}
environments: {staging: {}}
`
	cases := []struct {
		name       string
		input      string
		wantErrMsg string
	}{
		{name: "unsupported Valkey version", input: strings.Replace(ovhYAML, "valkeyVersion: 8.1", "valkeyVersion: 10.0", 1), wantErrMsg: "target.ovh.valkeyVersion"},
		{name: "MySQL plan node limit", input: strings.Replace(ovhYAML, "databaseNodeCount: 2", "databaseNodeCount: 3", 1), wantErrMsg: "target.ovh.databaseNodeCount"},
		{name: "Valkey plan minimum", input: strings.Replace(ovhYAML, "valkeyNodeCount: 2", "valkeyNodeCount: 1", 1), wantErrMsg: "target.ovh.valkeyNodeCount"},
		{name: "duplicate MKS zone", input: strings.Replace(ovhYAML, "zones: [eu-west-par-a, eu-west-par-b]", "zones: [eu-west-par-a, EU-WEST-PAR-A]", 1), wantErrMsg: "target.ovh.zones contains duplicate"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			file, err := Load([]byte(testCase.input))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), testCase.wantErrMsg) {
				t.Fatalf("Resolve() error = %v, want substring %q", err, testCase.wantErrMsg)
			}
		})
	}
}

func TestResolveAcceptsCurrentOVHValkeyVersions(t *testing.T) {
	const ovhYAML = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    region: EU-WEST-PAR
    zones: [eu-west-par-a]
    valkeyVersion: "%s"
defaults: {region: EU-WEST-PAR, preset: standard}
environments: {staging: {}}
`
	for _, version := range []string{"7.2", "8.0", "8.1", "9.0", "9.1"} {
		t.Run(version, func(t *testing.T) {
			file, err := Load([]byte(fmt.Sprintf(ovhYAML, version)))
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := file.Resolve("staging", ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := resolved.Config.Target.OVH.ValkeyVersion; got != version {
				t.Fatalf("ValkeyVersion = %q, want %q", got, version)
			}
		})
	}
}

func TestResolvePreservesAdvancedAWSEgressChoices(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {natMode: fck-nat, natTopology: multi-az, natReplacementMode: auto-scaling}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.NatMode != "fck-nat" || aws.NatTopology != "multi-az" || aws.NatReplacementMode != "auto-scaling" {
		t.Fatalf("advanced AWS egress choices = %#v", aws)
	}
	for _, path := range []string{"target.aws.natMode", "target.aws.natTopology", "target.aws.natReplacementMode"} {
		if effective.Provenance[path].Source != "project" {
			t.Fatalf("%s provenance = %#v", path, effective.Provenance[path])
		}
	}
}

func TestResolveEnvironmentOverlayCustomizesProviderAndExtensionSettings(t *testing.T) {
	input := strings.Replace(base,
		"target: {provider: aws, runtime: ecs-fargate}",
		"target: {provider: aws, runtime: ecs-fargate, aws: {natMode: nat-gateway}}",
		1,
	)
	input = strings.Replace(input,
		"    account: \"123\"\n",
		"    account: \"123\"\n    target:\n      aws:\n        natMode: fck-nat\n        natTopology: multi-az\n        natReplacementMode: auto-scaling\n    extensions:\n      community.network:\n        privateSubnetCount: 3\n",
		1,
	)

	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.NatMode != "fck-nat" || aws.NatTopology != "multi-az" || aws.NatReplacementMode != "auto-scaling" {
		t.Fatalf("overlay did not customize advanced AWS settings: %#v", aws)
	}
	if got := effective.Config.Extensions["community.network"].(map[string]any)["privateSubnetCount"]; got != 3 {
		t.Fatalf("community extension setting = %#v", got)
	}
	for _, path := range []string{
		"target.aws.natMode",
		"target.aws.natTopology",
		"target.aws.natReplacementMode",
		"extensions.community.network.privateSubnetCount",
	} {
		if got := effective.Provenance[path].Source; got != "environment staging" {
			t.Fatalf("%s provenance = %q", path, got)
		}
	}
}

func TestResolveMaterializesFckNatDefaults(t *testing.T) {
	for _, test := range []struct {
		name        string
		preset      string
		topology    string
		replacement string
	}{
		{name: "preview", preset: "preview", topology: "single-az", replacement: "none"},
		{name: "standard", preset: "standard", topology: "multi-az", replacement: "auto-scaling"},
		{name: "high-availability", preset: "high-availability", topology: "multi-az", replacement: "auto-scaling"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {natMode: fck-nat}}", 1)
			input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: eu-west-3, preset: "+test.preset+"}", 1)
			file, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			effective, err := file.Resolve("staging", ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			aws := effective.Config.Target.AWS
			if aws == nil || aws.NatTopology != test.topology || aws.NatReplacementMode != test.replacement || aws.NatInstanceType != "t4g.nano" {
				t.Fatalf("fck-nat defaults = %#v", aws)
			}
			for _, path := range []string{"target.aws.natReplacementMode", "target.aws.natInstanceType"} {
				if got := effective.Provenance[path].Source; got != "fck-nat defaults" {
					t.Fatalf("%s provenance = %q", path, got)
				}
			}
		})
	}
}

func TestResolvePreservesCustomFckNatInstanceType(t *testing.T) {
	input := strings.Replace(base,
		"target: {provider: aws, runtime: ecs-fargate}",
		"target: {provider: aws, runtime: ecs-fargate, aws: {natMode: fck-nat, natInstanceType: c6gn.medium}}",
		1,
	)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := effective.Config.Target.AWS.NatInstanceType; got != "c6gn.medium" {
		t.Fatalf("custom fck-nat instance type = %q", got)
	}
}

func TestResolveRejectsNonARMFckNatInstanceType(t *testing.T) {
	input := strings.Replace(base,
		"target: {provider: aws, runtime: ecs-fargate}",
		"target: {provider: aws, runtime: ecs-fargate, aws: {natMode: fck-nat, natInstanceType: t3.micro}}",
		1,
	)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "ARM64-compatible") {
		t.Fatalf("non-ARM fck-nat instance type was accepted: %v", err)
	}
}

func TestResolveRejectsIncompatibleAWSNATReplacement(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {natMode: nat-gateway, natReplacementMode: auto-scaling}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "only supported with fck-nat") {
		t.Fatalf("incompatible NAT replacement was accepted: %v", err)
	}
}

func TestResolveUsesMagento246RDSMariaDBCompatibilityDefault(t *testing.T) {
	input := withVersions(base, "2.4.6-p15", "8.2")
	input = strings.Replace(input, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Target.AWS == nil {
		t.Fatal("AWS target was not materialized")
	}
	if got := effective.Config.Target.AWS.Catalog.DatabaseEngine; got != "rds-mariadb" {
		t.Fatalf("Magento 2.4.6 database engine = %q, want rds-mariadb", got)
	}
	if got := effective.Config.Target.AWS.Catalog.Versions.MariaDB; got != "10.11.13" {
		t.Fatalf("Magento 2.4.6 RDS MariaDB default = %q, want 10.11.13", got)
	}
}

func TestResolveRejectsUnsupportedAWSServiceVersionBeforePlanning(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {searchMode: serverless, versions: {openSearch: OpenSearch_2.19}}}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "OpenSearch") {
		t.Fatalf("unsupported AWS OpenSearch version was accepted: %v", err)
	}

	input = strings.Replace(input, "extensions:\n", "compatibility: {allowUnsupported: true}\nextensions:\n", 1)
	f, err = Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err != nil {
		t.Fatalf("explicit compatibility exception was rejected: %v", err)
	}
}

func TestResolveDoesNotInventAWSTargetForMinimalConfig(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Target.AWS != nil {
		t.Fatal("minimal config unexpectedly received an AWS target")
	}
}

func TestResolvePreservesExplicitOptionsWhenUsingDefaultPresets(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{
		Builtins:      map[string]any{"defaults": map[string]any{"region": "us-east-1"}},
		Compatibility: map[string]any{"compatibility": map[string]any{"allowUnsupported": true}},
		Overrides:     map[string]any{"defaults": map[string]any{"region": "us-west-2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Defaults.Region != "us-west-2" || !effective.Config.Compatibility.AllowUnsupported {
		t.Fatalf("explicit resolver options were lost: %#v", effective.Config)
	}
}

func TestStrictUnknownKey(t *testing.T) {
	_, err := Load([]byte(strings.Replace(base, "project: {name: shop}", "project: {name: shop, typo: true}", 1)))
	if err == nil || !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSeedDumpPathLoadsAndResolves(t *testing.T) {
	input := strings.Replace(base, "    account: \"123\"\n", "    account: \"123\"\n    seedDump: /tmp/fixture.sql\n", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatalf("Load with seedDump: %v", err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve with seedDump: %v", err)
	}
	if effective.Config.SeedDump != "/tmp/fixture.sql" {
		t.Fatalf("SeedDump = %q", effective.Config.SeedDump)
	}
}

func TestSeedDumpStatusYAMLFieldRejected(t *testing.T) {
	input := strings.Replace(base, "    account: \"123\"\n", "    account: \"123\"\n    seedDumpStatus: recorded\n", 1)
	_, err := Load([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "field seedDumpStatus not found") {
		t.Fatalf("seedDumpStatus must remain journal-only, got: %v", err)
	}
}

func TestUnknownExtensionKeysAllowed(t *testing.T) {
	if _, err := Load([]byte(base)); err != nil {
		t.Fatal(err)
	}
}

func TestBuildHooksAreTypedAndValidated(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	input = strings.Replace(input, "build:\n  php: \"8.3\"", "build:\n  php: \"8.3\"\n  hooks:\n    build.prepare:\n      phase: build\n      relationship: before\n      target: build\n      command:\n        executable: composer\n        arguments: [run-script, prepare]", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := f.ResolveBuild()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Build.Hooks["build.prepare"].Command == nil {
		t.Fatal("hook command was not decoded")
	}
}

func TestBuildToolchainRequirementsAreTypedAndValidated(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	input = strings.Replace(
		input,
		"  composer: {credentials: aws-secrets-manager://composer/auth}",
		"  extensions: [apcu, redis]\n  composer: {version: \"2.10\", credentials: aws-secrets-manager://composer/auth}",
		1,
	)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := f.ResolveBuild()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(spec.Build.Extensions, ",") != "apcu,redis" || spec.Build.Composer.Version != "2.10" {
		t.Fatalf("build toolchain requirements = %#v", spec.Build)
	}

	for _, test := range []struct {
		name    string
		input   string
		message string
	}{
		{
			name:    "uppercase extension",
			input:   strings.Replace(input, "[apcu, redis]", "[APCu, redis]", 1),
			message: "lowercase PHP extension",
		},
		{
			name:    "duplicate extension",
			input:   strings.Replace(input, "[apcu, redis]", "[apcu, apcu]", 1),
			message: "duplicate",
		},
		{
			name:    "unsupported Composer major",
			input:   strings.Replace(input, `version: "2.10"`, `version: "1.10"`, 1),
			message: "Composer 2",
		},
		{
			name:    "invalid Composer minimum",
			input:   strings.Replace(input, `version: "2.10"`, `version: "2.10+minimum"`, 1),
			message: "Composer 2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := Load([]byte(test.input))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.ResolveBuild(); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q validation error, got %v", test.message, err)
			}
		})
	}
}

func TestRuntimeDefaultsResolveComposerRequirementByRelease(t *testing.T) {
	tests := []struct {
		release string
		php     string
		want    string
	}{
		{release: "2.4.6-p15", php: "8.2", want: "2.2.26+"},
		{release: "2.4.7-p10", php: "8.2", want: "2.10"},
		{release: "2.4.8-p5", php: "8.3", want: "2.10"},
		{release: "2.4.9", php: "8.5", want: "2.10"},
	}
	for _, test := range tests {
		t.Run(test.release, func(t *testing.T) {
			input := strings.Replace(base, "2.4.8-p5", test.release, 1)
			input = strings.Replace(input, `php: "8.3"`, fmt.Sprintf(`php: "%s"`, test.php), 1)
			input = strings.Replace(input, "  composer: {credentials: aws-secrets-manager://composer/auth}", "  composer: {}", 1)
			input = strings.Replace(input, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
			input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
			file, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			spec, err := file.ResolveBuild()
			if err != nil {
				t.Fatal(err)
			}
			if got := spec.Build.Composer.Version; got != test.want {
				t.Fatalf("Composer default = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAdobeCommerceRequiresComposerSecretReference(t *testing.T) {
	input := strings.Replace(base, "edition: open-source", "edition: commerce", 1)
	input = strings.Replace(input, "  composer: {credentials: aws-secrets-manager://composer/auth}", "  composer: {}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Resolve("staging", ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "required for Adobe Commerce") {
		t.Fatalf("missing Commerce credentials were accepted: %v", err)
	}
}

func TestBuildHooksRejectUnsafeCommandsAndRetries(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	input = strings.Replace(input, "build:\n  php: \"8.3\"", "build:\n  php: \"8.3\"\n  hooks:\n    build.prepare:\n      phase: build\n      relationship: before\n      target: build\n      command:\n        executable: sh\n        arguments: [\"echo unsafe\"]\n      retries:\n        maxAttempts: 2", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.ResolveBuild(); err == nil || !strings.Contains(err.Error(), "executable") || !strings.Contains(err.Error(), "idempotent") {
		t.Fatalf("unexpected hook validation error: %v", err)
	}
}

func TestRejectsUnnamespacedExtension(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "vendor.example", "example", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "namespaced identifier") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInheritanceCycle(t *testing.T) {
	input := strings.Replace(base, "  shared:\n", "  loop: {inherits: staging}\n  shared:\n    inherits: loop\n", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCloudSESRejectsPlaintextCredential(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, port: 587, username: ses-smtp-user, credential: not-a-secret-ref}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("plaintext SES credential was accepted: %v", err)
	}
}

func TestCloudSESRejectsGCPSecretOnAWS(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, port: 587, username: ses-smtp-user, credential: gcp-secret-manager://projects/p/secrets/ses-smtp/versions/latest}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "aws-secrets-manager:// or ssm://") {
		t.Fatalf("GCP SES secret on AWS was accepted: %v", err)
	}
}

func TestCloudSESAcceptsGCPSecretOnGCP(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: p, region: europe-west1}}", 1)
	input = strings.Replace(input, "aws-secrets-manager://composer/auth", "gcp-secret-manager://projects/p/secrets/composer/versions/latest", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses, host: email-smtp.europe-west1.amazonaws.com, port: 587, username: ses-smtp-user, credential: gcp-secret-manager://projects/p/secrets/ses-smtp/versions/latest}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCloudSESRequiresHostPortUsernameCredential(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "email.ses") || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("incomplete SES config was accepted: %v", err)
	}
}

func TestCloudSESValidatesSecretReference(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, port: 587, username: ses-smtp-user, credential: aws-secrets-manager://magelift/ses-smtp}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effective.Config.Email.Mode != "ses" || effective.Config.Email.Host != "email-smtp.eu-west-3.amazonaws.com" || effective.Config.Email.Port != 587 || effective.Config.Email.Username != "ses-smtp-user" {
		t.Fatalf("ses config = %+v", effective.Config.Email)
	}
	if effective.Config.Email.Credential != "aws-secrets-manager://magelift/ses-smtp" {
		t.Fatalf("credential = %q", effective.Config.Email.Credential)
	}
}

func TestPreviewOmitsEmailDoesNotReuseProductionSES(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, port: 587, username: ses-smtp-user, credential: aws-secrets-manager://magelift/ses-smtp}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	input = strings.Replace(input, "extensions:", "  production:\n    class: production\n    account: \"123\"\n  preview:\n    inherits: production\n    class: preview\nextensions:", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	production, err := f.Resolve("production", ResolveOptions{})
	if err != nil {
		t.Fatalf("production: %v", err)
	}
	if production.Config.Email.Mode != "ses" || production.Config.Email.Credential != "aws-secrets-manager://magelift/ses-smtp" {
		t.Fatalf("production email = %+v", production.Config.Email)
	}
	preview, err := f.Resolve("preview", ResolveOptions{})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Config.Email.Mode != "disabled" || preview.Config.Email.Credential != "" {
		t.Fatalf("preview reused production email: %+v", preview.Config.Email)
	}
	if preview.Provenance["email.mode"].Source != "preview email default" {
		t.Fatalf("preview email provenance = %+v", preview.Provenance["email.mode"])
	}
}

func TestPreviewExplicitSESKeepsSecretReference(t *testing.T) {
	input := strings.Replace(base, "defaults: {region: eu-west-3, preset: preview}", "email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, port: 587, username: ses-smtp-user, credential: aws-secrets-manager://magelift/ses-prod}\ndefaults: {region: eu-west-3, preset: preview}", 1)
	input = strings.Replace(input, "extensions:", "  preview:\n    class: preview\n    email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, port: 587, username: ses-smtp-user, credential: aws-secrets-manager://magelift/ses-preview}\nextensions:", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("preview", ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effective.Config.Email.Credential != "aws-secrets-manager://magelift/ses-preview" {
		t.Fatalf("preview email = %+v", effective.Config.Email)
	}
}

func TestRejectsPlaintextSecret(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "aws-secrets-manager://composer/auth", "hunter2", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsUnsupportedOrMalformedSecretReference(t *testing.T) {
	for _, reference := range []string{"vault-secrets://composer/auth", "ssm://parameter?withDecryption=false"} {
		f, err := Load([]byte(strings.Replace(base, "aws-secrets-manager://composer/auth", reference, 1)))
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Resolve("staging", ResolveOptions{})
		if err == nil || !strings.Contains(err.Error(), "valid secret reference") {
			t.Fatalf("reference %q returned %v", reference, err)
		}
	}
}

func TestRejectsGCPSecretSchemeOnAWSTarget(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "aws-secrets-manager://composer/auth", "gcp-secret-manager://projects/p/secrets/s/versions/latest", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "aws-secrets-manager:// or ssm://") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcceptsGCPSecretSchemeOnGCPTarget(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: p, region: europe-west1}}", 1)
	input = strings.Replace(input, "aws-secrets-manager://composer/auth", "gcp-secret-manager://projects/p/secrets/s/versions/latest", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsAWSSecretSchemeOnGCPTarget(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: p, region: europe-west1}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "gcp-secret-manager://") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnvironmentNamesSorted(t *testing.T) {
	f, _ := Load([]byte(base))
	got := strings.Join(f.Environments(), ",")
	if got != "shared,staging" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvironmentForBranch(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "  staging:\n", "  staging:\n    branches: [main]\n", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if got, found, err := f.EnvironmentForBranch("main"); err != nil || !found || got != "staging" {
		t.Fatalf("got environment=%q found=%v error=%v", got, found, err)
	}
	if _, found, err := f.EnvironmentForBranch("feature/no-map"); err != nil || found {
		t.Fatalf("unexpected match: found=%v error=%v", found, err)
	}
}

func TestEnvironmentForBranchRejectsAmbiguousMapping(t *testing.T) {
	input := strings.Replace(base, "  shared:\n", "  shared:\n    branches: [main]\n", 1)
	input = strings.Replace(input, "  staging:\n", "  staging:\n    branches: [main]\n", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.EnvironmentForBranch("main"); err == nil || !strings.Contains(err.Error(), "multiple environments") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataRegionResidencyFailsClosedForForeignBackup(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: shop-prod, cloudSqlBackupEnabled: true, cloudSqlBackupRetentionCount: 8, cloudSqlBackupLocation: us-central1}}", 1)
	input = strings.Replace(input, "aws-secrets-manager://composer/auth", "gcp-secret-manager://composer/auth", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: europe-west1, preset: preview}\nresilience: {dataRegion: eu}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "resilience.dataRegion") || !strings.Contains(err.Error(), "cloudSqlBackupLocation") {
		t.Fatalf("expected residency refusal, got %v", err)
	}
}

func TestDataRegionAllowsMatchingEUBackup(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: shop-prod, cloudSqlBackupEnabled: true, cloudSqlBackupRetentionCount: 8, cloudSqlBackupLocation: europe-west1}}", 1)
	input = strings.Replace(input, "aws-secrets-manager://composer/auth", "gcp-secret-manager://composer/auth", 1)
	input = strings.Replace(input, "defaults: {region: eu-west-3, preset: preview}", "defaults: {region: europe-west1, preset: preview}\nresilience: {dataRegion: eu}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Resilience.DataRegion != "eu" {
		t.Fatalf("dataRegion = %q", effective.Config.Resilience.DataRegion)
	}
}

func TestSQSWithoutMagentoModuleFailsClosed(t *testing.T) {
	input := strings.Replace(base, "application: {edition: open-source, version: 2.4.8-p5, mode: integrated}", "application: {edition: open-source, version: 2.4.8-p5, mode: integrated, magento: {queueTransport: sqs}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "queueModule") || !strings.Contains(err.Error(), "queueMode") {
		t.Fatalf("expected SQS module refusal, got %v", err)
	}
}

func TestSQSModuleMissingFromComposerLockFailsClosed(t *testing.T) {
	input := strings.Replace(base, "application: {edition: open-source, version: 2.4.8-p5, mode: integrated}", "application: {edition: open-source, version: 2.4.8-p5, mode: integrated, magento: {queueTransport: sqs, queueModule: magelift/module-queue-sqs}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{ComposerLock: []byte(`{"packages":[{"name":"magento/product-community-edition"}]}`)})
	if err == nil || !strings.Contains(err.Error(), "composer.lock") || !strings.Contains(err.Error(), "magelift/module-queue-sqs") {
		t.Fatalf("expected lock refusal, got %v", err)
	}
}

func TestSQSWithLockedComposerPackageIsAllowed(t *testing.T) {
	input := strings.Replace(base, "application: {edition: open-source, version: 2.4.8-p5, mode: integrated}", "application: {edition: open-source, version: 2.4.8-p5, mode: integrated, magento: {queueTransport: sqs, queueModule: magelift/module-queue-sqs}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{ComposerLock: []byte(`{"packages":[{"name":"magelift/module-queue-sqs"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Application.Magento.QueueTransport != "sqs" || effective.Config.Application.Magento.QueueModule != "magelift/module-queue-sqs" {
		t.Fatalf("queue integration = %#v", effective.Config.Application.Magento)
	}
}

const managedEmailAWSBase = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target: {provider: aws, runtime: ecs-fargate}
defaults: {region: eu-west-3, preset: preview}
environments: {staging: {}}
`

const managedEmailScalewayBase = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target:
  provider: scaleway
  runtime: kapsule
  scaleway:
    projectId: 11111111-1111-1111-1111-111111111111
defaults: {region: fr-par, preset: standard}
environments: {staging: {}}
`

const managedEmailOVHBase = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
defaults: {region: GRA9, preset: standard}
environments: {staging: {}}
`

func TestManagedSESAcceptsDomainAndZone(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: ses, from: shop@example.invalid, managed: {domain: example.invalid, hostedZoneId: Z1234567890ABC}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	managed := effective.Config.Email.Managed
	if managed == nil || managed.Domain != "example.invalid" || managed.HostedZoneID != "Z1234567890ABC" {
		t.Fatalf("managed = %#v", effective.Config.Email.Managed)
	}
}

func TestManagedSESRequiresZone(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: ses, managed: {domain: example.invalid}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "managed.hostedZoneId") {
		t.Fatalf("missing zone was accepted: %v", err)
	}
}

func TestManagedSESRejectsBYOMix(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: ses, host: email-smtp.eu-west-3.amazonaws.com, managed: {domain: example.invalid, hostedZoneId: Z123}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("BYO/managed mix was accepted: %v", err)
	}
}

func TestManagedSESRequiresAWSProvider(t *testing.T) {
	input := strings.Replace(managedEmailScalewayBase, "environments: {staging: {}}", "email: {mode: ses, managed: {domain: example.invalid, hostedZoneId: Z123}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires target.provider aws") {
		t.Fatalf("managed SES off AWS was accepted: %v", err)
	}
}

func TestManagedSESRejectsOVHAccount(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: ses, managed: {domain: example.invalid, hostedZoneId: Z123, account: shop}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "applies only to ovh") {
		t.Fatalf("misplaced account was accepted: %v", err)
	}
}

func TestTEMAcceptsFrParDomain(t *testing.T) {
	input := strings.Replace(managedEmailScalewayBase, "environments: {staging: {}}", "email: {mode: tem, from: shop@example.invalid, managed: {domain: example.invalid}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effective.Config.Email.Managed == nil || effective.Config.Email.Managed.Domain != "example.invalid" {
		t.Fatalf("managed = %#v", effective.Config.Email.Managed)
	}
}

func TestTEMRequiresManagedBlock(t *testing.T) {
	input := strings.Replace(managedEmailScalewayBase, "environments: {staging: {}}", "email: {mode: tem}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires a managed block") {
		t.Fatalf("unmanaged TEM was accepted: %v", err)
	}
}

func TestTEMRejectsNonFrParRegion(t *testing.T) {
	input := strings.Replace(managedEmailScalewayBase, "defaults: {region: fr-par, preset: standard}", "defaults: {region: nl-1, preset: standard}", 1)
	input = strings.Replace(input, "environments: {staging: {}}", "email: {mode: tem, managed: {domain: example.invalid}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "only available in fr-par") {
		t.Fatalf("non-fr-par TEM was accepted: %v", err)
	}
}

func TestTEMRequiresScalewayProvider(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: tem, managed: {domain: example.invalid}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "only supported on scaleway") {
		t.Fatalf("TEM off Scaleway was accepted: %v", err)
	}
}

func TestOVHMailboxAcceptsDomainAndAccount(t *testing.T) {
	input := strings.Replace(managedEmailOVHBase, "environments: {staging: {}}", "email: {mode: ovh, from: shop@example.invalid, managed: {domain: example.invalid, account: shop}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	managed := effective.Config.Email.Managed
	if managed == nil || managed.Domain != "example.invalid" || managed.Account != "shop" {
		t.Fatalf("managed = %#v", effective.Config.Email.Managed)
	}
}

func TestOVHRequiresAccount(t *testing.T) {
	input := strings.Replace(managedEmailOVHBase, "environments: {staging: {}}", "email: {mode: ovh, managed: {domain: example.invalid}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "domain and account") {
		t.Fatalf("accountless OVH was accepted: %v", err)
	}
}

func TestOVHRequiresOVHProvider(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: ovh, managed: {domain: example.invalid, account: shop}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "only supported on ovh") {
		t.Fatalf("OVH mail off OVH was accepted: %v", err)
	}
}

func TestManagedBlockRejectedOnSMTP(t *testing.T) {
	input := strings.Replace(managedEmailAWSBase, "environments: {staging: {}}", "email: {mode: smtp, host: mail.example.invalid, port: 2525, managed: {domain: example.invalid}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires mode ses, tem, or ovh") {
		t.Fatalf("managed block on smtp was accepted: %v", err)
	}
}

func TestTEMRejectsHostedZoneID(t *testing.T) {
	input := strings.Replace(managedEmailScalewayBase, "environments: {staging: {}}", "email: {mode: tem, managed: {domain: example.invalid, hostedZoneId: Z123}}\nenvironments: {staging: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "applies only to ses") {
		t.Fatalf("misplaced hostedZoneId was accepted: %v", err)
	}
}
