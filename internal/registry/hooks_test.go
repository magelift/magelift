package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestRegisterHooksPopulatesAllHooks(t *testing.T) {
	hooks := RegisterHooks()
	if hooks.Close != nil {
		t.Cleanup(hooks.Close)
	}
	if hooks.NewComposerSecrets == nil {
		t.Error("NewComposerSecrets is nil")
	}
	if hooks.NewComposerGCPSecrets == nil {
		t.Error("NewComposerGCPSecrets is nil")
	}
	if hooks.NewLeftoverBackupDestroyer == nil {
		t.Error("NewLeftoverBackupDestroyer is nil")
	}
	if hooks.NewGCPCleanupProvider == nil {
		t.Error("NewGCPCleanupProvider is nil")
	}
	if hooks.NewCleanupProvider == nil {
		t.Error("NewCleanupProvider is nil")
	}
	if hooks.MediaEndpoint == nil {
		t.Error("MediaEndpoint is nil")
	}
}

func TestRegisterHooksNeedsNoCredentials(t *testing.T) {
	hooks := RegisterHooks()
	if hooks.Close != nil {
		t.Cleanup(hooks.Close)
	}
	if _, err := hooks.NewCleanupProvider(context.Background(), sdk.CleanupLedger{Provider: "unknown-cloud"}); err == nil || !strings.Contains(err.Error(), `restartable cleanup has no adapter for provider "unknown-cloud"`) {
		t.Fatalf("unknown provider err = %v", err)
	}
	if _, err := hooks.NewLeftoverBackupDestroyer(context.Background(), nil); err == nil || err.Error() != "leftover Cloud SQL backup destroy requires a GCP planned stack" {
		t.Fatalf("leftover destroyer err = %v", err)
	}
}

func TestRegisterHooksMediaEndpointReadsEnvironment(t *testing.T) {
	hooks := RegisterHooks()
	if hooks.Close != nil {
		t.Cleanup(hooks.Close)
	}
	t.Setenv("MAGELIFT_AWS_ENDPOINT_URL", "")
	endpoint, err := hooks.MediaEndpoint()
	if err != nil || endpoint != "" {
		t.Fatalf("unset endpoint = %q, %v", endpoint, err)
	}
	t.Setenv("MAGELIFT_AWS_ENDPOINT_URL", "https://example.com")
	_, err = hooks.MediaEndpoint()
	if err == nil || !strings.Contains(err.Error(), `host "example.com" is not loopback`) {
		t.Fatalf("non-loopback err = %v", err)
	}
}

func TestRegisterHooksCleanupProviderRejectsIncompleteGCPLedgerBeforeClientLookup(t *testing.T) {
	hooks := RegisterHooks()
	t.Cleanup(hooks.Close)
	_, err := hooks.NewCleanupProvider(context.Background(), sdk.CleanupLedger{Provider: "gcp"})
	if err == nil || !strings.Contains(err.Error(), "require a project") {
		t.Fatalf("error = %v", err)
	}
}

func TestRegisterHooksCleanupProviderDefersGCPDial(t *testing.T) {
	hooks := RegisterHooks()
	t.Cleanup(hooks.Close)
	provider, err := hooks.NewCleanupProvider(context.Background(), sdk.CleanupLedger{Provider: "gcp", Project: "example-gcp", Marker: "m"})
	if err != nil {
		t.Fatalf("construction must not dial: %v", err)
	}
	_, err = provider.Inventory(context.Background(), sdk.CleanupInventoryRequest{Marker: "m", Provider: "gcp", Project: "example-gcp"})
	if err == nil || !strings.Contains(err.Error(), "magelift.providers.lock") {
		t.Fatalf("use without an installed plugin err = %v", err)
	}
}
