package stack

import (
	"testing"

	"github.com/magelift/magelift/providers/gcp/database"
)

func TestPlanFromInputsMagento249CloudSQLTierEnterpriseFallback(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Application.Version = "2.4.9"
	in.ValkeyRequirement = "9"

	defaultSpec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if defaultSpec.Catalog.CloudSQLTier != database.DefaultMySQL84Tier {
		t.Fatalf("default Cloud SQL tier = %q, want %q", defaultSpec.Catalog.CloudSQLTier, database.DefaultMySQL84Tier)
	}
	if err := defaultSpec.Validate(); err != nil {
		t.Fatalf("default Magento 2.4.9 plan: %v", err)
	}

	in.Target.CloudSQLTier = "db-custom-2-7680"
	customSpec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	if customSpec.Catalog.CloudSQLTier != "db-custom-2-7680" {
		t.Fatalf("explicit Cloud SQL tier = %q", customSpec.Catalog.CloudSQLTier)
	}
	if err := customSpec.Validate(); err != nil {
		t.Fatalf("custom tier Magento 2.4.9 plan: %v", err)
	}
}
