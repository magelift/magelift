package stack

import (
	"net/netip"
	"testing"
	"time"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func validSpec() Spec {
	hostedZone := sdk.ExistingResourceRef{ID: "zone", Provider: "aws", Kind: sdk.ExistingDNSZone, ExternalID: "Z123456789"}
	certificate := sdk.ExistingResourceRef{ID: "certificate", Provider: "aws", Kind: sdk.ExistingCertificate, ExternalID: "arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000"}
	albCertificate := sdk.ExistingResourceRef{ID: "alb-certificate", Provider: "aws", Kind: sdk.ExistingCertificate, ExternalID: "arn:aws:acm:eu-west-3:123456789012:certificate/11111111-1111-1111-1111-111111111111"}
	return Spec{
		Identity:     Identity{Project: "shop", Environment: "preview-1", AccountID: "123456789012", Region: "eu-west-3", EnvironmentClass: "preview", Preset: sdk.PresetPreview},
		Application:  Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     Artifact{ImageDigest: "ghcr.io/acourtiol/shop@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CompatibilityStatus: "compatible", RequiredRuntimeCapabilities: []sdk.CapabilityID{sdk.CapabilityDatabaseMySQL, sdk.CapabilityCacheValkey}},
		Lifecycle:    Lifecycle{ExpiresAt: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC), MonthlyBudgetCents: 25000},
		Existing:     ExistingResources{HostedZone: &hostedZone, Certificate: &certificate, ALBCertificate: &albCertificate},
		Dependencies: Dependencies{KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555", CacheSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-cache-token", EncryptionKeyARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-encryption-key", DatabaseName: "magento", MasterUsername: "magento"},
		Policy:       NetworkPolicy{VPCCIDR: netip.MustParsePrefix("10.42.0.0/16"), AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, ApplicationDomain: "preview.example.com", MediaDomain: "media.preview.example.com", NatMode: NatModeGateway},
		Catalog:      CatalogSelection{Version: "2026.07-preview.1", DatabaseEngine: DatabaseEngineAuroraMySQL, SearchMode: SearchModeServerless, Aurora: AuroraPreviewProfile{MinimumACU: 0, MaximumACU: 4, AutoPauseSeconds: 900, EngineSupportsAutoPause: true}, Valkey: ValkeyPreviewProfile{NodeType: "cache.t4g.micro"}, Search: SearchPreviewProfile{MaximumIndexingOCU: 2, MaximumSearchOCU: 2, AcceptColdStarts: true}, Fargate: FargatePreviewProfile{CPU: 512, MemoryMiB: 1024, DesiredCount: 1}, Retention: RetentionProfile{LogDays: 7, BackupDays: 1, ArtifactDays: 7}, Versions: ServiceVersions{AuroraMySQL: "8.0.mysql_aurora.3.12", Valkey: "8.1", OpenSearch: "OpenSearch_3.1", RabbitMQ: "3.13"}},
	}
}

func TestSpecValidateAcceptsExplicitPreviewPlan(t *testing.T) {
	if err := validSpec().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSpecValidateRejectsGuessedOrUnsafeInputs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Spec)
	}{
		{name: "missing catalog", edit: func(spec *Spec) { spec.Catalog.Version = "" }},
		{name: "missing encryption key", edit: func(spec *Spec) { spec.Dependencies.EncryptionKeyARN = "" }},
		{name: "expired preview", edit: func(spec *Spec) { spec.Lifecycle.ExpiresAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{name: "mutable image", edit: func(spec *Spec) { spec.Artifact.ImageDigest = "ghcr.io/acourtiol/shop:latest" }},
		{name: "unsupported artifact capability", edit: func(spec *Spec) {
			spec.Artifact.RequiredRuntimeCapabilities = []sdk.CapabilityID{sdk.CapabilityQueueRabbitMQ}
		}},
		{name: "missing certificate", edit: func(spec *Spec) { spec.Existing.Certificate = nil }},
		{name: "invalid subnet policy", edit: func(spec *Spec) { spec.Policy.VPCCIDR = netip.MustParsePrefix("10.42.1.1/16") }},
		{name: "incomplete existing database", edit: func(spec *Spec) {
			ref := sdk.ExistingResourceRef{ID: "database", Provider: "aws", Kind: sdk.ExistingDatabase, ExternalID: "db-magento"}
			spec.Existing.Database = &ref
			spec.Existing.DatabaseEndpoint = "magento.xxxxx.eu-west-3.rds.amazonaws.com"
		}},
		{name: "wrong existing database kind", edit: func(spec *Spec) {
			ref := sdk.ExistingResourceRef{ID: "database", Provider: "aws", Kind: sdk.ExistingNetwork, ExternalID: "db-magento"}
			spec.Existing.Database = &ref
			spec.Existing.DatabaseSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master"
			spec.Existing.DatabaseEndpoint = "magento.xxxxx.eu-west-3.rds.amazonaws.com"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.edit(&spec)
			if err := spec.Validate(); err == nil {
				t.Fatal("invalid stack plan was accepted")
			}
		})
	}
}

func TestSpecValidateAcceptsExistingDatabase(t *testing.T) {
	spec := validSpec()
	ref := sdk.ExistingResourceRef{ID: "database", Provider: "aws", Kind: sdk.ExistingDatabase, ExternalID: "db-magento"}
	spec.Existing.Database = &ref
	spec.Existing.DatabaseSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master"
	spec.Existing.DatabaseEndpoint = "magento.xxxxx.eu-west-3.rds.amazonaws.com"
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSpecValidateAcceptsRDSManagedMasterUserSecretARN(t *testing.T) {
	spec := validSpec()
	ref := sdk.ExistingResourceRef{ID: "database", Provider: "aws", Kind: sdk.ExistingDatabase, ExternalID: "mladopt-mysql"}
	spec.Existing.Database = &ref
	spec.Existing.DatabaseSecretARN = "arn:aws:secretsmanager:eu-north-1:111122223333:secret:rds!db-EXAMPLE-secret"
	spec.Existing.DatabaseEndpoint = "mladopt-mysql.xxxxx.eu-north-1.rds.amazonaws.com"
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSpecValidateAllowsThreeZoneStandardQueueLayout(t *testing.T) {
	spec := validSpec()
	spec.Identity.EnvironmentClass = "staging"
	spec.Identity.Preset = sdk.PresetStandard
	spec.Lifecycle.ExpiresAt = time.Time{}
	spec.Policy.AvailabilityZones = []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}
	spec.Catalog.Valkey.ReplicaCount = 1
	spec.Catalog.Fargate.DesiredCount = 2
	spec.Catalog.SearchMode = SearchModeProvisioned
	spec.Dependencies.SessionSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-session-token"
	spec.Dependencies.QueueSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-queue-token"
	spec.Catalog.AuroraProvisioned = AuroraProvisionedProfile{InstanceClass: "db.r8g.large", InstanceCount: 2}
	spec.Catalog.SearchProvisioned = SearchProvisionedProfile{InstanceType: "m7g.large.search", InstanceCount: 2, EBSVolumeType: "gp3", EBSVolumeSizeGiB: 200}
	spec.Catalog.RabbitMQ = RabbitMQProfile{InstanceType: "mq.m7g.large"}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
}
