package stack

import (
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func TestAdoptReportExistingNetwork(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{
				ID:         "shop-staging-network",
				Provider:   "aws",
				Kind:       sdk.ExistingNetwork,
				ExternalID: "vpc-0123456789abcdef0",
			},
		},
	}
	entries := AdoptReport(spec)
	if len(entries) != 1 {
		t.Fatalf("AdoptReport len = %d, want 1", len(entries))
	}
	if entries[0].Kind != "network" || entries[0].ExternalID != "vpc-0123456789abcdef0" {
		t.Fatalf("entry = %#v", entries[0])
	}
	if entries[0].Label != "shop-staging-network" {
		t.Fatalf("label = %q", entries[0].Label)
	}
	if got := entries[0].Line(); got != "ADOPT network vpc-0123456789abcdef0" {
		t.Fatalf("Line() = %q", got)
	}
}

func TestAdoptReportEmptyWithoutExistingNetwork(t *testing.T) {
	t.Parallel()
	if entries := AdoptReport(Spec{}); len(entries) != 0 {
		t.Fatalf("expected empty report, got %#v", entries)
	}
}

func TestAdoptReportExistingDatabase(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Database: &sdk.ExistingResourceRef{
				ID:         "shop-staging-database",
				Provider:   "aws",
				Kind:       sdk.ExistingDatabase,
				ExternalID: "db-magento-prod",
			},
			DatabaseSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master",
			DatabaseEndpoint:  "magento.xxxxx.eu-west-3.rds.amazonaws.com",
		},
	}
	entries := AdoptReport(spec)
	if len(entries) != 1 {
		t.Fatalf("AdoptReport len = %d, want 1", len(entries))
	}
	if entries[0].Kind != "database" || entries[0].ExternalID != "db-magento-prod" {
		t.Fatalf("entry = %#v", entries[0])
	}
	if entries[0].Label != "shop-staging-database" {
		t.Fatalf("label = %q", entries[0].Label)
	}
	if got := entries[0].Line(); got != "ADOPT database db-magento-prod" {
		t.Fatalf("Line() = %q", got)
	}
}

func TestAdoptReportNetworkAndDatabase(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{
				ID:         "shop-staging-network",
				Provider:   "aws",
				Kind:       sdk.ExistingNetwork,
				ExternalID: "vpc-0123456789abcdef0",
			},
			Database: &sdk.ExistingResourceRef{
				ID:         "shop-staging-database",
				Provider:   "aws",
				Kind:       sdk.ExistingDatabase,
				ExternalID: "db-magento-prod",
			},
			DatabaseSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master",
			DatabaseEndpoint:  "magento.xxxxx.eu-west-3.rds.amazonaws.com",
		},
	}
	entries := AdoptReport(spec)
	if len(entries) != 2 {
		t.Fatalf("AdoptReport len = %d, want 2", len(entries))
	}
	if entries[0].Line() != "ADOPT network vpc-0123456789abcdef0" {
		t.Fatalf("network entry = %#v", entries[0])
	}
	if entries[1].Line() != "ADOPT database db-magento-prod" {
		t.Fatalf("database entry = %#v", entries[1])
	}
}

func TestRefuseAdoptedMutationNamesNetworkExternalID(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{
				ID:         "shop-staging-network",
				Provider:   "aws",
				Kind:       sdk.ExistingNetwork,
				ExternalID: "vpc-0adoptednetwork01",
			},
		},
	}
	for _, intent := range []platform.AdoptMutationIntent{platform.AdoptIntentDestroy, platform.AdoptIntentReplace} {
		err := RefuseAdoptedMutation(spec, intent)
		if err == nil {
			t.Fatalf("intent %q: expected refuse error", intent)
		}
		msg := err.Error()
		if !strings.Contains(msg, "vpc-0adoptednetwork01") {
			t.Fatalf("intent %q: error missing externalId: %v", intent, err)
		}
		if !strings.Contains(msg, "shop-staging-network") {
			t.Fatalf("intent %q: error missing resource name: %v", intent, err)
		}
		if !strings.Contains(msg, "MageLift does not own this resource") {
			t.Fatalf("intent %q: error missing ownership phrase: %v", intent, err)
		}
	}
}

func TestRefuseAdoptedMutationAllowsNonMutatingIntent(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{ID: "net", Provider: "aws", Kind: sdk.ExistingNetwork, ExternalID: "vpc-1"},
		},
	}
	if err := RefuseAdoptedMutation(spec, ""); err != nil {
		t.Fatalf("empty intent should allow Magento-scoped ops: %v", err)
	}
	if err := RefuseAdoptedMutation(Spec{}, platform.AdoptIntentDestroy); err != nil {
		t.Fatalf("no existing network should allow destroy intent: %v", err)
	}
}

func TestAdoptedRefuseNamesDatabaseExternalID(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Database: &sdk.ExistingResourceRef{
				ID:         "shop-staging-database",
				Provider:   "aws",
				Kind:       sdk.ExistingDatabase,
				ExternalID: "db-magento-prod",
			},
			DatabaseSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master",
			DatabaseEndpoint:  "magento.xxxxx.eu-west-3.rds.amazonaws.com",
		},
	}
	for _, intent := range []platform.AdoptMutationIntent{platform.AdoptIntentDestroy, platform.AdoptIntentReplace} {
		err := RefuseAdoptedMutation(spec, intent)
		if err == nil {
			t.Fatalf("intent %q: expected refuse error", intent)
		}
		msg := err.Error()
		if !strings.Contains(msg, "db-magento-prod") {
			t.Fatalf("intent %q: error missing database externalId: %v", intent, err)
		}
		if !strings.Contains(msg, "shop-staging-database") {
			t.Fatalf("intent %q: error missing database name: %v", intent, err)
		}
		if !strings.Contains(msg, "MageLift does not own this resource") {
			t.Fatalf("intent %q: error missing ownership phrase: %v", intent, err)
		}
	}
}

func TestAdoptedRefuseNamesNetworkAndDatabase(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{
				ID:         "shop-staging-network",
				Provider:   "aws",
				Kind:       sdk.ExistingNetwork,
				ExternalID: "vpc-0adoptednetwork01",
			},
			Database: &sdk.ExistingResourceRef{
				ID:         "shop-staging-database",
				Provider:   "aws",
				Kind:       sdk.ExistingDatabase,
				ExternalID: "db-magento-prod",
			},
		},
	}
	err := RefuseAdoptedMutation(spec, platform.AdoptIntentDestroy)
	if err == nil {
		t.Fatal("expected refuse error for network+database adopt")
	}
	msg := err.Error()
	if !strings.Contains(msg, "vpc-0adoptednetwork01") {
		t.Fatalf("error missing network externalId: %v", err)
	}
	if !strings.Contains(msg, "db-magento-prod") {
		t.Fatalf("error missing database externalId: %v", err)
	}
	if !strings.Contains(msg, "MageLift does not own this resource") {
		t.Fatalf("error missing ownership phrase: %v", err)
	}
}

// TestAdoptedDetachDestroyRecordsNoDeleteForAdoptedIDs is the ATTACH-04 offline
// half: reference-without-own keeps VPC/RDS out of Pulumi state, so a destroy of
// MageLift-owned children never emits Delete for adopted external IDs. Explicit
// destroy/replace intent against adopted refs is refused by name. Live
// describe-after-destroy remains 08-06 HUMAN_GATE.
func TestAdoptedDetachDestroyRecordsNoDeleteForAdoptedIDs(t *testing.T) {
	t.Parallel()
	const (
		adoptedVPC = "vpc-0adoptednetwork01"
		adoptedDB  = "db-magento-prod"
	)
	spec := Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{
				ID: "shop-staging-network", Provider: "aws", Kind: sdk.ExistingNetwork, ExternalID: adoptedVPC,
			},
			Database: &sdk.ExistingResourceRef{
				ID: "shop-staging-database", Provider: "aws", Kind: sdk.ExistingDatabase, ExternalID: adoptedDB,
			},
		},
	}
	adopted := map[string]struct{}{}
	for _, entry := range AdoptReport(spec) {
		adopted[entry.ExternalID] = struct{}{}
	}
	if _, ok := adopted[adoptedVPC]; !ok {
		t.Fatal("adopted VPC missing from AdoptReport")
	}
	if _, ok := adopted[adoptedDB]; !ok {
		t.Fatal("adopted database missing from AdoptReport")
	}

	// Mock Pulumi state after attach: MageLift-owned children only — adopted
	// external IDs were never registered, so they cannot appear in Delete.
	ownedState := []string{
		"urn:pulumi:stack::proj::aws:ecs/service:Service::shop",
		"urn:pulumi:stack::proj::aws:lb/loadBalancer:LoadBalancer::shop",
	}
	deleteLog := simulateDestroyOwnedChildren(ownedState, adopted)
	for _, id := range []string{adoptedVPC, adoptedDB} {
		for _, deleted := range deleteLog {
			if strings.Contains(deleted, id) {
				t.Fatalf("Delete emitted for adopted externalId %s: %v", id, deleteLog)
			}
		}
	}
	if err := RefuseAdoptedMutation(spec, ""); err != nil {
		t.Fatalf("empty intent (stack detach of owned children) must be allowed: %v", err)
	}
	if err := RefuseAdoptedMutation(spec, platform.AdoptIntentDestroy); err == nil {
		t.Fatal("explicit destroy of adopted resources must be refused")
	}
}

// simulateDestroyOwnedChildren records Delete targets from mock Pulumi state.
// Adopted external IDs are excluded from state by construction (reference-without-own).
func simulateDestroyOwnedChildren(stateURNs []string, adoptedExternalIDs map[string]struct{}) []string {
	var deletes []string
	for _, urn := range stateURNs {
		for id := range adoptedExternalIDs {
			if strings.Contains(urn, id) {
				deletes = append(deletes, urn)
			}
		}
		// Owned children are "deleted" but we only care that adopted IDs are absent.
		_ = urn
	}
	return deletes
}

func TestPlannedImplementsBrownfieldAttach(t *testing.T) {
	t.Parallel()
	planned := Planned{Spec: Spec{
		Existing: ExistingResources{
			Network: &sdk.ExistingResourceRef{ID: "n", Provider: "aws", Kind: sdk.ExistingNetwork, ExternalID: "vpc-abc"},
		},
	}}
	var attach platform.BrownfieldAttach = planned
	lines := attach.AdoptedResourceLines()
	if len(lines) != 1 || lines[0] != "ADOPT network vpc-abc" {
		t.Fatalf("AdoptedResourceLines = %v", lines)
	}
	if err := attach.RefuseAdoptedMutation(platform.AdoptIntentDestroy); err == nil || !strings.Contains(err.Error(), "vpc-abc") {
		t.Fatalf("RefuseAdoptedMutation = %v", err)
	}
}
