// Package cli exposes the supported custom-binary entry point for MageLift
// community extensions.
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cleanup"
	internalcli "github.com/magelift/magelift/internal/cli"
	awssecrets "github.com/magelift/magelift/internal/cloud/aws/secrets"
	"github.com/magelift/magelift/internal/cloud/gcp/naming"
	gcpresilience "github.com/magelift/magelift/internal/cloud/gcp/resilience"
	gcpsecrets "github.com/magelift/magelift/internal/cloud/gcp/secrets"
	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/registry"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

// New returns a command with the first-party MageLift modules registered.
func New() (*cobra.Command, error) {
	return NewWithExtensions()
}

// ExitCode preserves the CLI's stable user-error exit code contract for
// custom binaries.
func ExitCode(err error) int {
	return internalcli.ExitCode(err)
}

// Execute runs a command with graceful SIGINT/SIGTERM cancellation.
func Execute(command *cobra.Command) error {
	return internalcli.Execute(command)
}

// NewWithExtensions returns a command with first-party modules plus the
// explicitly linked community modules. It never loads extensions from disk.
func NewWithExtensions(extensions ...sdk.Module) (*cobra.Command, error) {
	modules, err := registry.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("register first-party modules: %w", err)
	}
	for _, extension := range extensions {
		if err := modules.RegisterPublicModule(extension); err != nil {
			return nil, err
		}
	}
	return internalcli.NewWithModulesAndHooks(modules, firstPartyHooks()), nil
}

func firstPartyHooks() internalcli.Hooks {
	return internalcli.Hooks{
		NewComposerSecrets: func(ctx context.Context, region string) (internalcli.ComposerSecretProvider, error) {
			return awssecrets.New(ctx, region)
		},
		NewComposerGCPSecrets: func(ctx context.Context) (internalcli.ComposerSecretProvider, error) {
			store, err := gcpsecrets.NewStore(ctx)
			if err != nil {
				return nil, err
			}
			return gcpComposerSecretAdapter{store: store}, nil
		},
		NewLeftoverBackupDestroyer: newGCPCloudSQLLeftoverBackupDestroyer,
		NewGCPCleanupProvider:      newGCPCloudSQLCleanupProvider,
	}
}

type gcpComposerSecretAdapter struct {
	store interface {
		GetSecretValue(context.Context, string) ([]byte, error)
	}
}

func (a gcpComposerSecretAdapter) GetSecretValue(ctx context.Context, id string) ([]byte, error) {
	return a.store.GetSecretValue(ctx, id)
}

func (a gcpComposerSecretAdapter) GetParameter(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("GCP Secret Manager does not resolve Parameter Store references")
}

type gcpCloudSQLLeftoverBackupDestroyer struct {
	api      *gcpresilience.NativeAPI
	instance string
}

func (d gcpCloudSQLLeftoverBackupDestroyer) Destroy(ctx context.Context) ([]string, error) {
	return d.api.DeleteLeftoverCloudSQLBackupsForInstance(ctx, d.instance)
}

func newGCPCloudSQLLeftoverBackupDestroyer(ctx context.Context, planned platform.PlannedStack) (internalcli.LeftoverBackupDestroyer, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("leftover Cloud SQL backup destroy requires a GCP planned stack")
	}
	project := strings.TrimSpace(gcpPlanned.Spec.Identity.GCPProject)
	instance := naming.CloudSQLInstance(gcpPlanned.Spec.Identity.Project, gcpPlanned.Spec.Identity.Environment)
	if project == "" || instance == "" {
		return nil, fmt.Errorf("leftover Cloud SQL backup destroy requires a GCP project and instance name")
	}
	api, err := gcpresilience.NewGCPCloudSQLNativeAPI(ctx, gcpresilience.NativeAPIConfig{Project: project})
	if err != nil {
		return nil, fmt.Errorf("create GCP Cloud SQL leftover backup client: %w", err)
	}
	return gcpCloudSQLLeftoverBackupDestroyer{api: api, instance: instance}, nil
}

func newGCPCloudSQLCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
	if strings.TrimSpace(ledger.Project) == "" {
		return nil, fmt.Errorf("GCP cleanup ledgers require a project")
	}
	native, err := gcpresilience.NewGCPCloudSQLNativeAPI(ctx, gcpresilience.NativeAPIConfig{Project: ledger.Project})
	if err != nil {
		return nil, err
	}
	return cleanup.GCPCloudSQLProvider{SQL: native.SQL(), Project: ledger.Project, Marker: ledger.Marker}, nil
}
