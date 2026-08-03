package stack

import (
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/sdk/v1"
)

func TestPlanFromConfigMapsExplicitAWSInputs(t *testing.T) {
	cfg := deploymentConfig()
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("planned spec is invalid: %v", err)
	}
	if spec.Identity.Preset != v1.PresetStandard || spec.Identity.AccountID != cfg.Account {
		t.Fatalf("identity was not mapped: %#v", spec.Identity)
	}
	if spec.Artifact.ImageDigest != cfg.Target.AWS.ImageDigest {
		t.Fatalf("image digest = %q", spec.Artifact.ImageDigest)
	}
	if !spec.Lifecycle.ExpiresAt.Equal(time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("expiration = %s", spec.Lifecycle.ExpiresAt)
	}
	if spec.Catalog.AuroraProvisioned.InstanceClass != "db.r7g.large" || spec.Catalog.SearchProvisioned.InstanceCount != 2 {
		t.Fatalf("catalog was not mapped: %#v", spec.Catalog)
	}
	if spec.Existing.Certificate.ExternalID != cfg.Target.AWS.CloudFrontCertificateARN || spec.Existing.ALBCertificate.ExternalID != cfg.Target.AWS.ALBCertificateARN {
		t.Fatal("certificate references were not mapped")
	}
}

func TestPlanFromConfigMapsExistingNetworkInputs(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Network:          &config.AWSExistingResource{Provider: "aws", Kind: "network", ExternalID: "vpc-existing"},
		PublicSubnetIDs:  []string{"subnet-public-a", "subnet-public-b"},
		PrivateSubnetIDs: []string{"subnet-private-a", "subnet-private-b"},
		DataSubnetIDs:    []string{"subnet-data-a", "subnet-data-b"},
	}
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Existing.Network == nil || spec.Existing.Network.ExternalID != "vpc-existing" || len(spec.Existing.PrivateSubnetIDs) != 2 {
		t.Fatalf("existing network was not mapped: %#v", spec.Existing)
	}
}

func TestPlanFromConfigRejectsIncompleteExistingNetwork(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Network:          &config.AWSExistingResource{Provider: "aws", Kind: "network", ExternalID: "vpc-existing"},
		PrivateSubnetIDs: []string{"subnet-private-a"},
	}
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "existing network requires") {
		t.Fatalf("incomplete existing network was accepted: %v", err)
	}
}

func TestPlanFromConfigMapsExistingDatabaseInputs(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Database: &config.AWSExistingDatabase{
			Provider:   "aws",
			Kind:       "database",
			ExternalID: "db-magento-prod",
			SecretARN:  "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master",
			Endpoint:   "magento.xxxxx.eu-west-3.rds.amazonaws.com",
		},
	}
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Existing.Database == nil {
		t.Fatal("existing database was not mapped")
	}
	if spec.Existing.Database.Kind != v1.ExistingDatabase || spec.Existing.Database.Provider != "aws" {
		t.Fatalf("database ref kind/provider = %#v", spec.Existing.Database)
	}
	if spec.Existing.Database.ExternalID != "db-magento-prod" {
		t.Fatalf("database external ID = %q", spec.Existing.Database.ExternalID)
	}
	if spec.Existing.DatabaseSecretARN != cfg.Target.AWS.Existing.Database.SecretARN {
		t.Fatalf("database secret ARN = %q", spec.Existing.DatabaseSecretARN)
	}
	if spec.Existing.DatabaseEndpoint != cfg.Target.AWS.Existing.Database.Endpoint {
		t.Fatalf("database endpoint = %q", spec.Existing.DatabaseEndpoint)
	}
}

func TestPlanFromConfigRejectsIncompleteExistingDatabase(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Database: &config.AWSExistingDatabase{
			Provider:   "aws",
			Kind:       "database",
			ExternalID: "db-magento-prod",
			Endpoint:   "magento.xxxxx.eu-west-3.rds.amazonaws.com",
		},
	}
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "existing database requires") {
		t.Fatalf("incomplete existing database was accepted: %v", err)
	}

	cfg = deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Database: &config.AWSExistingDatabase{
			Provider:   "aws",
			Kind:       "database",
			ExternalID: "db-magento-prod",
			SecretARN:  "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master",
		},
	}
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "existing database requires") {
		t.Fatalf("incomplete existing database was accepted: %v", err)
	}
}

func TestPlanFromConfigRejectsWrongExistingDatabaseKind(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Database: &config.AWSExistingDatabase{
			Provider:   "aws",
			Kind:       "network",
			ExternalID: "db-magento-prod",
			SecretARN:  "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master",
			Endpoint:   "magento.xxxxx.eu-west-3.rds.amazonaws.com",
		},
	}
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "existing database must be an AWS database reference") {
		t.Fatalf("wrong database kind was accepted: %v", err)
	}
}

func TestPlanFromConfigKeepsExistingNetworkOnly(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.AWS.Existing = config.AWSExistingResources{
		Network:          &config.AWSExistingResource{Provider: "aws", Kind: "network", ExternalID: "vpc-existing"},
		PublicSubnetIDs:  []string{"subnet-public-a", "subnet-public-b"},
		PrivateSubnetIDs: []string{"subnet-private-a", "subnet-private-b"},
		DataSubnetIDs:    []string{"subnet-data-a", "subnet-data-b"},
	}
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Existing.Network == nil || spec.Existing.Database != nil {
		t.Fatalf("network-only existing plan drifted: %#v", spec.Existing)
	}
}

func TestPlanFromConfigRejectsGuessedDeploymentInputs(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Target.Provider = "gcp"
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("unsupported target was accepted: %v", err)
	}

	cfg = deploymentConfig()
	cfg.Target.AWS = nil
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "target.aws is required") {
		t.Fatalf("unexpected missing AWS input error: %v", err)
	}

	cfg = deploymentConfig()
	cfg.Class = ""
	if _, err := PlanFromConfig(cfg, "production"); err == nil || !strings.Contains(err.Error(), "class must be explicit") {
		t.Fatalf("unexpected class error: %v", err)
	}

	cfg = deploymentConfig()
	cfg.ExpiresAt = "tomorrow"
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("unexpected expiration error: %v", err)
	}
}

func TestPlanFromConfigRunsTargetValidationBeforePlanning(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Class = "production"
	cfg.Preset = "preview"
	cfg.Defaults.Preset = "preview"
	cfg.ExpiresAt = "2026-12-01T12:00:00+00:00"
	cfg.Target.AWS.Catalog.Aurora = config.AWSCatalogAurora{MinimumACU: 0, MaximumACU: 4, AutoPauseSeconds: 900, EngineSupportsAutoPause: true}
	cfg.Target.AWS.Catalog.Valkey = config.AWSCatalogValkey{NodeType: "cache.t4g.micro"}
	cfg.Target.AWS.Catalog.Search = config.AWSCatalogSearch{MaximumIndexingOCU: 2, MaximumSearchOCU: 2, AcceptColdStarts: true}
	cfg.Target.AWS.Catalog.Fargate = config.AWSCatalogFargate{CPU: 512, MemoryMiB: 1024, DesiredCount: 1}

	if _, err := PlanFromConfig(cfg, "production"); err == nil || !strings.Contains(err.Error(), "production environments cannot use the preview preset") {
		t.Fatalf("target validation was not enforced: %v", err)
	}
}

func TestPlanFromConfigAllowsExpiredPreviewOnlyForDestroy(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Class = "preview"
	cfg.Preset = "preview"
	cfg.Defaults.Preset = "preview"
	cfg.ExpiresAt = "2020-01-01T00:00:00Z"
	cfg.Target.AWS.Catalog.Aurora = config.AWSCatalogAurora{MinimumACU: 0, MaximumACU: 4, AutoPauseSeconds: 900, EngineSupportsAutoPause: true}
	cfg.Target.AWS.Catalog.Valkey = config.AWSCatalogValkey{NodeType: "cache.t4g.micro"}
	cfg.Target.AWS.Catalog.Search = config.AWSCatalogSearch{MaximumIndexingOCU: 2, MaximumSearchOCU: 2, AcceptColdStarts: true}
	cfg.Target.AWS.Catalog.Fargate = config.AWSCatalogFargate{CPU: 512, MemoryMiB: 1024, DesiredCount: 1}
	if _, err := PlanFromConfig(cfg, "preview"); err == nil || !strings.Contains(err.Error(), "expiration") {
		t.Fatalf("expired preview was accepted for normal planning: %v", err)
	}
	if _, err := PlanFromConfigWithOptions(cfg, "preview", PlanOptions{AllowExpiredPreview: true}); err != nil {
		t.Fatalf("expired preview was not accepted for destroy planning: %v", err)
	}
}

func TestPlanFromConfigUsesResolvedDefaultPresetWhenEnvironmentDoesNotOverride(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Preset = ""
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Identity.Preset != v1.PresetStandard {
		t.Fatalf("preset = %q", spec.Identity.Preset)
	}
}

func TestPlanFromConfigRejectsUnsupportedAWSServiceCombination(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Application.Version = "2.4.9"
	cfg.Target.AWS.Catalog.Versions.OpenSearch = "OpenSearch_2.19"
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "OpenSearch") {
		t.Fatalf("unexpected compatibility error: %v", err)
	}
	cfg.Compatibility.AllowUnsupported = true
	if _, err := PlanFromConfig(cfg, "staging"); err != nil {
		t.Fatalf("explicit compatibility override was rejected: %v", err)
	}
}

func TestPlanFromConfigAcceptsCurrentAWSServiceVersions(t *testing.T) {
	cfg := deploymentConfig()
	cfg.Application.Version = "2.4.9"
	cfg.Target.AWS.Catalog.Versions.Valkey = "9.1"
	cfg.Target.AWS.Catalog.Versions.RabbitMQ = "4.2"
	cfg.Target.AWS.Catalog.RabbitMQ.InstanceType = "mq.m7g.large"
	if _, err := PlanFromConfig(cfg, "staging"); err != nil {
		t.Fatalf("current AWS service versions were rejected: %v", err)
	}

	cfg.Target.AWS.Catalog.RabbitMQ.InstanceType = "mq.m5.large"
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "mq.m7g") {
		t.Fatalf("RabbitMQ 4.2 instance constraint was not enforced: %v", err)
	}

	cfg.Target.AWS.Catalog.RabbitMQ.InstanceType = ""
	if err := validateAWSServiceCompatibility(cfg); err != nil {
		t.Fatalf("empty RabbitMQ instance type should skip the mq.m7g gate: %v", err)
	}
}

func deploymentConfig() config.Config {
	return config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application:   config.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &config.AWSTarget{
			KMSKeyARN:                "arn:aws:kms:eu-west-3:123456789012:key/01234567-89ab-cdef-0123-456789abcdef",
			HostedZoneID:             "Z123456789",
			CloudFrontCertificateARN: "arn:aws:acm:us-east-1:123456789012:certificate/01234567-89ab-cdef-0123-456789abcdef",
			ALBCertificateARN:        "arn:aws:acm:eu-west-3:123456789012:certificate/abcdef01-2345-6789-abcd-ef0123456789",
			SNSTopicARN:              "arn:aws:sns:eu-west-3:123456789012:deployments",
			ImageDigest:              "ghcr.io/magelift/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			CacheSecretARN:           "arn:aws:secretsmanager:eu-west-3:123456789012:secret:cache-token",
			SessionSecretARN:         "arn:aws:secretsmanager:eu-west-3:123456789012:secret:session-token",
			QueueSecretARN:           "arn:aws:secretsmanager:eu-west-3:123456789012:secret:queue-token",
			EncryptionKeySecretARN:   "arn:aws:secretsmanager:eu-west-3:123456789012:secret:encryption-key",
			DatabaseName:             "magento",
			MasterUsername:           "magento_admin",
			VPCCIDR:                  "10.20.0.0/16",
			AvailabilityZones:        []string{"eu-west-3a", "eu-west-3b"},
			MediaDomain:              "media.shop.example",
			Catalog: config.AWSCatalog{
				Version:   "catalog-2026-07-01",
				Aurora:    config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
				Valkey:    config.AWSCatalogValkey{NodeType: "cache.r7g.large", ReplicaCount: 1},
				Search:    config.AWSCatalogSearch{InstanceType: "r7g.large.search", InstanceCount: 2, EBSVolumeType: "gp3", EBSVolumeSizeGiB: 100},
				RabbitMQ:  config.AWSCatalogRabbitMQ{InstanceType: "mq.m7g.large"},
				Fargate:   config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 2},
				Retention: config.AWSCatalogRetention{LogDays: 30, BackupDays: 7, ArtifactDays: 30},
				Versions:  config.AWSCatalogVersions{AuroraMySQL: "8.0.mysql_aurora.3.12", Valkey: "8.1", OpenSearch: "OpenSearch_3.1", RabbitMQ: "3.13"},
			},
		}},
		Defaults:           config.Defaults{Region: "eu-west-3", Preset: "standard"},
		Account:            "123456789012",
		Class:              "staging",
		Preset:             "standard",
		Domain:             "shop.example",
		ExpiresAt:          "2026-12-01T12:00:00+00:00",
		MonthlyBudgetCents: 250000,
	}
}
