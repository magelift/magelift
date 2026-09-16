package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/magelift/magelift/internal/cleanup"
	internalcli "github.com/magelift/magelift/internal/cli"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	awssecrets "github.com/magelift/magelift/internal/cloud/aws/secrets"
	"github.com/magelift/magelift/internal/cloud/gcp/naming"
	gcpresilience "github.com/magelift/magelift/internal/cloud/gcp/resilience"
	gcpsecrets "github.com/magelift/magelift/internal/cloud/gcp/secrets"
	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	ovhresilience "github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// RegisterHooks returns the first-party provider hook constructors used by the
// released CLI. This is the single core-adjacent place allowed to import
// provider hook packages for construction. No constructor runs here; each hook
// builds its client only when the CLI invokes it, so registration needs no
// credentials.
func RegisterHooks() internalcli.Hooks {
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
		NewCleanupProvider:         newFirstPartyCleanupProvider,
		MediaEndpoint: func() (string, error) {
			return awsendpoint.FromEnv()
		},
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

func newFirstPartyCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
	switch ledger.Provider {
	case "gcp":
		if strings.TrimSpace(ledger.Project) == "" {
			return nil, errors.New("GCP cleanup ledgers require a project")
		}
		sqlAPI, err := gcpresilience.NewCloudSQLAPI(ctx)
		if err != nil {
			return nil, fmt.Errorf("create GCP Cloud SQL cleanup client: %w", err)
		}
		return cleanup.GCPCloudSQLProvider{SQL: sqlAPI, Project: strings.TrimSpace(ledger.Project), Marker: ledger.Marker}, nil
	case "ovh":
		return newOVHDatabaseCleanupProvider(ctx, ledger)
	case "scaleway":
		return newScalewayCleanupProvider(ctx, ledger)
	default:
		return nil, cleanup.UnsupportedProvider(ledger.Provider)
	}
}

func newOVHDatabaseCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
	if strings.TrimSpace(ledger.Profile) == "" || strings.TrimSpace(ledger.Region) == "" || strings.TrimSpace(ledger.Project) == "" {
		return nil, errors.New("OVHcloud cleanup ledgers require profile, region, and project")
	}
	client, err := ovhresilience.NewOVHClientFromProfile(ledger.Profile)
	if err != nil {
		return nil, err
	}
	native, err := ovhresilience.NewOVHDatabaseNativeAPI(ctx, ovhresilience.NativeAPIConfig{
		DatabaseProjectID: ledger.Project, DatabaseEngine: "mysql", DatabaseRegion: ledger.Region,
		DatabaseDeletePollInterval: time.Second,
	}, client)
	if err != nil {
		return nil, err
	}
	return cleanup.OVHDatabaseProvider{Database: native.Database(), Engine: "mysql", Marker: ledger.Marker}, nil
}

func newScalewayCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
	if strings.TrimSpace(ledger.Profile) == "" || strings.TrimSpace(ledger.Region) == "" || strings.TrimSpace(ledger.Project) == "" {
		return nil, errors.New("Scaleway cleanup ledgers require profile, region, and project")
	}
	config, err := scw.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load Scaleway profile configuration: %w", err)
	}
	profileConfig, err := config.GetProfile(ledger.Profile)
	if err != nil {
		return nil, fmt.Errorf("load Scaleway profile %q: %w", ledger.Profile, err)
	}
	profileClient, err := scw.NewClient(scw.WithProfile(profileConfig))
	if err != nil {
		return nil, fmt.Errorf("construct Scaleway profile client: %w", err)
	}
	accessKey, accessKeyOK := profileClient.GetAccessKey()
	secretKey, secretKeyOK := profileClient.GetSecretKey()
	if !accessKeyOK || !secretKeyOK || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, errors.New("selected Scaleway profile does not contain an access key and secret key")
	}
	native, err := resilience.NewScalewayDatabaseNativeAPI(ctx, resilience.NativeAPIConfig{
		Region:            ledger.Region,
		DatabaseRegion:    ledger.Region,
		DatabaseProjectID: ledger.Project,
		Credentials:       credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		RetentionDays:     1,
	}, scw.WithProfile(profileConfig), scw.WithDefaultProjectID(ledger.Project), scw.WithDefaultRegion(scw.Region(ledger.Region)))
	if err != nil {
		return nil, err
	}
	return cleanup.ScalewayDatabaseProvider{Database: native.Database(), Marker: ledger.Marker}, nil
}
