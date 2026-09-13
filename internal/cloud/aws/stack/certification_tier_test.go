package stack

import (
	"testing"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestCertificationTierFollowsEvidenceTuple(t *testing.T) {
	t.Parallel()
	preview := Spec{
		Identity:    Identity{Preset: sdk.PresetPreview},
		Application: Application{Version: "2.4.9", WebRuntime: "nginx-fpm"},
		Catalog: CatalogSelection{
			DatabaseEngine: DatabaseEngineRDSMySQL,
			QueueMode:      QueueModeDB,
			SearchMode:     SearchModeDisabled,
		},
	}
	if got := (Planned{Spec: preview}).CertificationTier(); got != platform.TierCertified {
		t.Fatalf("preview fargate 2.4.9 = %s, want certified", got)
	}
	mi := preview
	mi.Catalog.Fargate.ComputeMode = "managed-instances"
	if got := (Planned{Spec: mi}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("managed instances = %s, want experimental", got)
	}
	ha := preview
	ha.Identity.Preset = sdk.PresetHighAvailability
	if got := (Planned{Spec: ha}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("HA preset = %s, want experimental", got)
	}
	mq := preview
	mq.Catalog.QueueMode = QueueModeAmazonMQ
	if got := (Planned{Spec: mq}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("amazon-mq = %s, want experimental", got)
	}
	aurora := preview
	aurora.Catalog.DatabaseEngine = DatabaseEngineAuroraMySQL
	if got := (Planned{Spec: aurora}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("aurora-mysql = %s, want experimental", got)
	}
	search := preview
	search.Catalog.SearchMode = SearchModeProvisioned
	if got := (Planned{Spec: search}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("provisioned search = %s, want experimental", got)
	}
	franken := preview
	franken.Application.WebRuntime = "frankenphp-classic"
	if got := (Planned{Spec: franken}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("frankenphp-classic = %s, want experimental", got)
	}
}
