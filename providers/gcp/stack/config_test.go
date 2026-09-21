package stack

import (
	"strings"
	"testing"
	"time"

	gcpruntime "github.com/magelift/magelift/providers/gcp/runtime"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

func TestPlanFromInputsMapsExplicitGCPInputs(t *testing.T) {
	in := gcpDeploymentInputs()
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("planned spec is invalid: %v", err)
	}
	if spec.Identity.Preset != sdk.PresetStandard || spec.Identity.GCPProject != in.Target.Project {
		t.Fatalf("identity was not mapped: %#v", spec.Identity)
	}
	if spec.Identity.Region != "europe-west1" {
		t.Fatalf("region = %q", spec.Identity.Region)
	}
	if spec.Artifact.ImageDigest != in.Target.ImageDigest {
		t.Fatalf("image digest = %q", spec.Artifact.ImageDigest)
	}
	if spec.Policy.NetworkCIDR != "10.20.0.0/16" {
		t.Fatalf("network CIDR = %q", spec.Policy.NetworkCIDR)
	}
	if len(spec.Policy.Zones) != 2 || spec.Policy.Zones[0] != "europe-west1-b" {
		t.Fatalf("zones = %#v", spec.Policy.Zones)
	}
	if spec.Catalog.CloudSQLTier != "db-perf-optimized-N-2" || spec.Catalog.DesiredWebReplicas != 2 {
		t.Fatalf("catalog was not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.CloudSQLAvailability != "REGIONAL" || spec.Catalog.QueueMode != "rabbitmq" {
		t.Fatalf("standard production catalog = %#v", spec.Catalog)
	}
	if spec.Catalog.CloudSQLDatabaseVersion != "MYSQL_8_4" {
		t.Fatalf("Cloud SQL database version = %q, want MYSQL_8_4", spec.Catalog.CloudSQLDatabaseVersion)
	}
	if spec.Catalog.MemorystoreEngineVersion != "VALKEY_8_0" {
		t.Fatalf("Memorystore engine version = %q, want VALKEY_8_0 for the 2.4.8 compatibility probe", spec.Catalog.MemorystoreEngineVersion)
	}
	if !spec.Catalog.EnableCloudArmor {
		t.Fatal("standard GCP plans should keep Cloud Armor enabled by default")
	}
	if spec.Catalog.OpenSearchImage == "" || spec.Catalog.RabbitMQImage == "" {
		t.Fatalf("GCP service images must have release-aware defaults: %#v", spec.Catalog)
	}
	if spec.Catalog.AutopilotMemoryRequest != gcpruntime.DefaultApplicationMemoryRequest {
		t.Fatalf("GCP application memory default = %q, want %q", spec.Catalog.AutopilotMemoryRequest, gcpruntime.DefaultApplicationMemoryRequest)
	}
	if spec.Dependencies.DatabaseName != "magento" || spec.Dependencies.MasterUsername != "magento" {
		t.Fatalf("dependencies were not mapped: %#v", spec.Dependencies)
	}
}

func TestPlanFromInputsMapsAdvancedGCPInfrastructureSettings(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Runtime = gcptarget.RuntimeStandardID
	gcp := &in.Target
	gcp.CloudSQLTier = "db-c4a-highmem-2"
	gcp.CloudSQLAvailability = "ZONAL"
	memorystoreReplicas := 3
	memorystoreShardCount := 4
	gcp.MemorystoreShardCount = &memorystoreShardCount
	gcp.MemorystoreReplicas = &memorystoreReplicas
	gcp.MemorystoreMode = "CLUSTER"
	gcp.MemorystoreZoneDistributionMode = "SINGLE_ZONE"
	gcp.MemorystoreZone = "europe-west1-b"
	cloudSQLBackupEnabled := true
	cloudSQLBinaryLogEnabled := true
	cloudSQLBackupRetentionCount := 14
	cloudSQLTransactionLogRetention := 7
	cloudSQLDeletionProtection := true
	memorystoreDeletionProtection := true
	memorystorePSCConnectionLimit := 4
	gcp.CloudSQLBackupEnabled = &cloudSQLBackupEnabled
	gcp.CloudSQLBinaryLogEnabled = &cloudSQLBinaryLogEnabled
	gcp.CloudSQLBackupRetentionCount = &cloudSQLBackupRetentionCount
	gcp.CloudSQLTransactionLogRetention = &cloudSQLTransactionLogRetention
	gcp.CloudSQLBackupStartTime = "03:30"
	gcp.CloudSQLBackupLocation = "europe-west1"
	gcp.CloudSQLDeletionProtection = &cloudSQLDeletionProtection
	gcp.MemorystoreDeletionProtection = &memorystoreDeletionProtection
	gcp.MemorystorePSCConnectionLimit = &memorystorePSCConnectionLimit
	gcp.OpenSearchMode = "disabled"
	gcp.QueueMode = "rabbitmq"
	gcp.QueueReplicas = 4
	gcp.QueueConsumerCount = 6
	gcp.KubernetesVersion = "1.32"
	gcp.ReleaseChannel = "STABLE"
	gcp.ClusterIPv4CIDR = "10.64.0.0/16"
	gcp.ServicesIPv4CIDR = "10.65.0.0/20"
	gcp.StandardNodeType = "n2-standard-8"
	gcp.StandardNodeCount = 4
	gcp.StandardNodeMinCount = 3
	gcp.StandardNodeMaxCount = 6
	gcp.StandardNodeDiskType = "pd-ssd"
	gcp.StandardNodeDiskSizeGiB = 200
	gcp.StandardNodeImageType = "UBUNTU_CONTAINERD"
	gcp.StandardNodeSpot = true
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Catalog.CloudSQLTier != "db-c4a-highmem-2" || spec.Catalog.CloudSQLAvailability != "ZONAL" || spec.Catalog.MemorystoreShardCount != 4 || spec.Catalog.MemorystoreReplicas != 3 || spec.Catalog.MemorystoreMode != "CLUSTER" || spec.Catalog.MemorystoreZoneDistributionMode != "SINGLE_ZONE" || spec.Catalog.MemorystoreZone != "europe-west1-b" {
		t.Fatalf("managed service settings were not mapped: %#v", spec.Catalog)
	}
	if !spec.Catalog.CloudSQLBackupEnabled || !spec.Catalog.CloudSQLBinaryLogEnabled || spec.Catalog.CloudSQLBackupRetentionCount != 14 || spec.Catalog.CloudSQLTransactionLogRetention != 7 || spec.Catalog.CloudSQLBackupStartTime != "03:30" || spec.Catalog.CloudSQLBackupLocation != "europe-west1" || !spec.Catalog.CloudSQLDeletionProtection || !spec.Catalog.MemorystoreDeletionProtection || spec.Catalog.MemorystorePSCConnectionLimit != 4 {
		t.Fatalf("managed service protection settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.SearchMode != "disabled" || spec.Catalog.QueueMode != "rabbitmq" || spec.Catalog.QueueReplicas != 4 || spec.Catalog.QueueConsumerCount != 6 {
		t.Fatalf("workload settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.KubernetesVersion != "1.32" || spec.Catalog.ReleaseChannel != "STABLE" || spec.Catalog.ClusterIPv4CIDR != "10.64.0.0/16" || spec.Catalog.ServicesIPv4CIDR != "10.65.0.0/20" {
		t.Fatalf("GKE cluster settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.StandardNodeType != "n2-standard-8" || spec.Catalog.StandardNodeCount != 4 || spec.Catalog.StandardNodeMinCount != 3 || spec.Catalog.StandardNodeMaxCount != 6 || spec.Catalog.StandardNodeDiskType != "pd-ssd" || spec.Catalog.StandardNodeDiskSizeGiB != 200 || spec.Catalog.StandardNodeImageType != "UBUNTU_CONTAINERD" || !spec.Catalog.StandardNodeSpot {
		t.Fatalf("GKE Standard node settings were not mapped: %#v", spec.Catalog)
	}
}

func TestPlanFromInputsPreservesExplicitZeroMemorystoreReplicas(t *testing.T) {
	in := gcpDeploymentInputs()
	memorystoreReplicas := 0
	in.Target.MemorystoreReplicas = &memorystoreReplicas
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Catalog.MemorystoreReplicas != 0 {
		t.Fatalf("explicit zero Memorystore replicas were replaced by a preset default: %#v", spec.Catalog)
	}
}

func TestPlanFromInputsMapsReleaseAwareCloudSQLVersions(t *testing.T) {
	for _, test := range []struct {
		version string
		valkey  string
		want    string
	}{
		{version: "2.4.6-p15", valkey: "8.1", want: "MYSQL_8_0"},
		{version: "2.4.7-p10", valkey: "8.1", want: "MYSQL_8_4"},
		{version: "2.4.8-p5", valkey: "8.1", want: "MYSQL_8_4"},
		{version: "2.4.9", valkey: "9", want: "MYSQL_8_4"},
	} {
		t.Run(test.version, func(t *testing.T) {
			in := gcpDeploymentInputs()
			in.Application.Version = test.version
			in.ValkeyRequirement = test.valkey
			spec, err := PlanFromInputs(in)
			if err != nil {
				t.Fatal(err)
			}
			if spec.Catalog.CloudSQLDatabaseVersion != test.want {
				t.Fatalf("Cloud SQL database version = %q, want %q", spec.Catalog.CloudSQLDatabaseVersion, test.want)
			}
		})
	}
}

func TestPlanFromInputsSelectsValkeyNineForMagento249(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Application.Version = "2.4.9"
	in.ValkeyRequirement = "9"
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := spec.Catalog.MemorystoreEngineVersion; got != "VALKEY_9_0" {
		t.Fatalf("Memorystore engine version = %q, want VALKEY_9_0", got)
	}
}

func TestPlanFromInputsRejectsMismatchedMemorystoreEngineVersion(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Application.Version = "2.4.9"
	in.ValkeyRequirement = "9"
	in.Target.MemorystoreEngineVersion = "VALKEY_8_0"
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "VALKEY_9_0") {
		t.Fatalf("mismatched Memorystore engine version was accepted: %v", err)
	}
}

func TestPlanFromInputsAcceptsValkeyNineOnePreview(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Application.Version = "2.4.9"
	in.ValkeyRequirement = "9"
	in.Target.MemorystoreEngineVersion = "VALKEY_9_1"
	in.AllowUnsupported = false
	in.Envelope.Environment = "preview"
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := spec.Catalog.MemorystoreEngineVersion; got != "VALKEY_9_1" {
		t.Fatalf("Memorystore engine version = %q, want VALKEY_9_1", got)
	}
}

func TestPlanFromInputsSupportsGKEStandard(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Runtime = gcptarget.RuntimeStandardID
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("planned standard spec is invalid: %v", err)
	}
	if spec.Identity.Runtime != gcptarget.RuntimeStandardID {
		t.Fatalf("runtime = %q, want gke-standard", spec.Identity.Runtime)
	}
}

func TestPlanFromInputsMapsExplicitCloudArmorOverride(t *testing.T) {
	in := gcpDeploymentInputs()
	enableCloudArmor := false
	in.Target.EnableCloudArmor = &enableCloudArmor
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Catalog.EnableCloudArmor {
		t.Fatal("explicit enableCloudArmor=false was ignored")
	}
}

func TestPlanFromInputsMapsExplicitGCPServiceImages(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Target.OpenSearchImage = "registry.example/opensearch:3.3.0@sha256:" + strings.Repeat("a", 64)
	in.Target.RabbitMQImage = "registry.example/rabbitmq:4.2-management@sha256:" + strings.Repeat("b", 64)
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Catalog.OpenSearchImage != in.Target.OpenSearchImage || spec.Catalog.RabbitMQImage != in.Target.RabbitMQImage {
		t.Fatalf("service images were not mapped: %#v", spec.Catalog)
	}
}

func TestPlanFromInputsRejectsEmptyTarget(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Target = providerschema.GCPTarget{}
	in.Envelope.Region = ""
	in.Envelope.Preset = ""
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "must be resolved") {
		t.Fatalf("unexpected empty target error: %v", err)
	}
}

func TestPlanFromInputsRejectsGuessedDeploymentInputs(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Envelope.EnvironmentClass = ""
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "class must be explicit") {
		t.Fatalf("missing class was accepted: %v", err)
	}

	in = gcpDeploymentInputs()
	in.Envelope.ExpiresAt = "not-a-timestamp"
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("invalid expiration was accepted: %v", err)
	}
}

func TestPlanFromInputsAllowsExpiredPreviewOnlyForDestroy(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Envelope.Environment = "preview"
	in.Envelope.EnvironmentClass = "preview"
	in.Envelope.Preset = "preview"
	in.Envelope.ExpiresAt = "2020-01-01T00:00:00Z"
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired preview was accepted for normal planning: %v", err)
	}
	in.AllowExpiredPreview = true
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatalf("expired preview was not accepted for destroy planning: %v", err)
	}
	if !spec.AllowExpiredPreview {
		t.Fatal("destroy planning did not record AllowExpiredPreview on the spec; the Pulumi program would reject the destroy")
	}
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("strict validation stopped rejecting the expired preview: %v", err)
	}
	if err := spec.ValidateAllowExpiredPreview(); err != nil {
		t.Fatalf("teardown validation rejected the expired preview: %v", err)
	}
	in.Target.ImageDigest = "not-a-digest"
	if _, err := PlanFromInputs(in); err == nil {
		t.Fatal("allow-expired planning forgave a non-expiry defect")
	}
}

func TestPlanFromInputsFallsBackToEnvelopeRegion(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Target.Region = ""
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Identity.Region != "europe-west1" || spec.Identity.Preset != sdk.PresetStandard {
		t.Fatalf("envelope fallback was not applied: %#v", spec.Identity)
	}
}

func gcpDeploymentInputs() PlanInputs {
	return PlanInputs{
		Envelope: sdk.Envelope{
			Project: "shop", Environment: "staging", Region: "europe-west1",
			EnvironmentClass: "staging", Preset: "standard",
			ExpiresAt: time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		Application: sdk.Application{
			Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm",
		},
		Target: providerschema.GCPTarget{
			Project:             "example-gcp-project",
			Region:              "europe-west1",
			NetworkCIDR:         "10.20.0.0/16",
			ImageDigest:         "ghcr.io/magelift/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			EncryptionKeySecret: "magento-crypt-key",
		},
		Runtime:           gcptarget.RuntimeAutopilotID,
		ValkeyRequirement: "8.1",
		AllowUnsupported:  true,
	}
}

func TestPlanFromInputsAccountOnlySkipsImageAndEncryptionKey(t *testing.T) {
	in := gcpDeploymentInputs()
	in.AccountOnly = true
	in.Target.ImageDigest = ""
	in.Target.EncryptionKeySecret = ""
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.AccountOnly {
		t.Fatal("account-only plan was not recorded")
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("deploy validation accepted an account-only spec that has no image")
	}
	in.AccountOnly = false
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "artifact image digest") {
		t.Fatalf("deploy plan error = %v", err)
	}
	in.Target.ImageDigest = "ghcr.io/magelift/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	spec, err = PlanFromInputs(in)
	if err != nil {
		t.Fatalf("deploy plan without an operator encryption key: %v", err)
	}
	if spec.Dependencies.EncryptionKeySecret != "" {
		t.Fatalf("generated-key plan stored an operator secret ID %q", spec.Dependencies.EncryptionKeySecret)
	}
}

func TestPlanFromInputsMapsEmailRelay(t *testing.T) {
	t.Parallel()
	in := gcpDeploymentInputs()
	in.Application.Email = sdk.EmailSettings{
		Mode: "smtp", Host: "smtp.example.com", Port: 587, Username: "mailer",
		From: "shop@example.com", Credential: "gcp-secret-manager://projects/example-gcp-project/secrets/smtp-password/versions/latest",
	}
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Email.Enabled() || spec.Email.Host != "smtp.example.com" || spec.Email.Port != 587 {
		t.Fatalf("email = %#v", spec.Email)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("spec with email is invalid: %v", err)
	}
}

func TestPlanFromInputsValidatesEmailRelay(t *testing.T) {
	t.Parallel()
	valid := sdk.EmailSettings{
		Mode: "smtp", Host: "smtp.example.com", Port: 587, Username: "mailer",
		Credential: "gcp-secret-manager://projects/example-gcp-project/secrets/smtp-password/versions/latest",
	}
	cases := []struct {
		name   string
		mutate func(*sdk.EmailSettings)
		want   string
	}{
		{"bad mode", func(e *sdk.EmailSettings) { e.Mode = "tem" }, "not servable on GCP"},
		{"missing host", func(e *sdk.EmailSettings) { e.Host = "" }, "email host is required"},
		{"bad port", func(e *sdk.EmailSettings) { e.Port = 0 }, "email port must be between"},
		{"missing username", func(e *sdk.EmailSettings) { e.Username = "" }, "email username is required"},
		{"missing credential", func(e *sdk.EmailSettings) { e.Credential = "" }, "email credential"},
		{"wrong scheme", func(e *sdk.EmailSettings) { e.Credential = "aws-secrets-manager://smtp" }, "must use gcp-secret-manager"},
		{"short name", func(e *sdk.EmailSettings) { e.Credential = "gcp-secret-manager://smtp-password" }, "must name projects/"},
		{"json field", func(e *sdk.EmailSettings) { e.Credential += "?jsonField=password" }, "jsonField extraction is not supported"},
		{"disabled with fields", func(e *sdk.EmailSettings) { e.Mode = "disabled" }, "require a sending mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			email := valid
			tc.mutate(&email)
			in := gcpDeploymentInputs()
			in.Application.Email = email
			_, err := PlanFromInputs(in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
