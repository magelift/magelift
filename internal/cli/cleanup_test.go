package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

type fakeCleanupProvider struct {
	resources []sdk.CleanupInventoryResource
	deleted   []string
}

func (fake *fakeCleanupProvider) Inventory(context.Context, sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error) {
	return append([]sdk.CleanupInventoryResource(nil), fake.resources...), nil
}

func (fake *fakeCleanupProvider) Delete(_ context.Context, resource sdk.CleanupResource) error {
	fake.deleted = append(fake.deleted, resource.Kind+":"+resource.Identity)
	remaining := make([]sdk.CleanupInventoryResource, 0, len(fake.resources))
	for _, item := range fake.resources {
		if item.Identity == resource.Identity {
			continue
		}
		remaining = append(remaining, item)
	}
	fake.resources = remaining
	return nil
}

func TestCleanupClaimRecordAndReconcileRemovesOwnedResource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.json")
	provider := &fakeCleanupProvider{}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.newCleanupProvider = func(context.Context, sdk.CleanupLedger) (cleanupProvider, error) {
		return provider, nil
	}
	claim := newCommandWithOptions(o)
	claim.SetArgs([]string{
		"--output", "json", "cleanup", "claim",
		"--ledger", path,
		"--run-id", "run-cli",
		"--marker", "magelift/scaleway/rdb-recovery/run-cli",
		"--provider", "scaleway",
		"--region", "fr-par",
		"--project", "11111111-1111-1111-1111-111111111111",
		"--profile", "default",
		"--kind", "rdb-instance",
		"--role", "source",
		"--name", "magelift-rdb-run-cli",
	})
	if err := claim.Execute(); err != nil {
		t.Fatal(err)
	}

	provider.resources = []sdk.CleanupInventoryResource{{
		Kind: "rdb-instance", Role: sdk.CleanupRoleSource, Name: "magelift-rdb-run-cli", Identity: "instance-1", Owned: true, Live: true,
	}}
	out.Reset()
	record := newCommandWithOptions(o)
	record.SetArgs([]string{"--output", "json", "cleanup", "record", "--ledger", path, "--kind", "rdb-instance", "--name", "magelift-rdb-run-cli", "--identity", "instance-1"})
	if err := record.Execute(); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	plan := newCommandWithOptions(o)
	plan.SetArgs([]string{"--output", "json", "cleanup", "plan", "--ledger", path})
	if err := plan.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "pending"`) {
		t.Fatalf("plan = %s", out.String())
	}

	out.Reset()
	reconcile := newCommandWithOptions(o)
	reconcile.SetArgs([]string{"--yes", "--output", "json", "cleanup", "reconcile", "--ledger", path})
	if err := reconcile.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "complete"`) || len(provider.deleted) != 1 {
		t.Fatalf("reconcile = %s deleted=%v", out.String(), provider.deleted)
	}
}

func TestCleanupClaimInitializesEmptyLedgerFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &fakeCleanupProvider{}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.newCleanupProvider = func(context.Context, sdk.CleanupLedger) (cleanupProvider, error) {
		return provider, nil
	}
	claim := newCommandWithOptions(o)
	claim.SetArgs([]string{
		"--output", "json", "cleanup", "claim",
		"--ledger", path,
		"--run-id", "run-empty",
		"--marker", "magelift/gcp/sql-cleanup/run-empty",
		"--provider", "gcp",
		"--region", "europe-west1",
		"--project", "demo",
		"--kind", "cloudsql-instance",
		"--role", "source",
		"--name", "magelift-cledgr-run-empty",
	})
	if err := claim.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupReconcileRequiresYes(t *testing.T) {
	var out bytes.Buffer
	cmd := newCommandWithOptions(testOptions(&out, &fakeTerminal{interactive: false}))
	cmd.SetArgs([]string{"cleanup", "reconcile", "--ledger", "missing.json"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
}
