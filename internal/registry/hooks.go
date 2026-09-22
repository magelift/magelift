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
	ovhresilience "github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// RegisterHooks returns the first-party provider hook constructors used by the
// released CLI. GCP hooks dial the verified provider plugin lazily (one
// session shared across hooks per CLI invocation); other providers construct
// in-process clients. No constructor runs here; each hook builds its client
// only when the CLI invokes it, so registration needs no credentials.
func RegisterHooks() internalcli.Hooks {
	dialer := &providerhost.CachedDialer{Dial: dialGCP}
	return internalcli.Hooks{
		Close: dialer.Close,
		NewComposerSecrets: func(ctx context.Context, region string) (internalcli.ComposerSecretProvider, error) {
			return awssecrets.New(ctx, region)
		},
		NewComposerGCPSecrets: func(ctx context.Context) (internalcli.ComposerSecretProvider, error) {
			client, err := dialer.Do(ctx)
			if err != nil {
				return nil, err
			}
			store, err := providerhost.NewSecretStore(client, sdk.Envelope{})
			if err != nil {
				return nil, err
			}
			return gcpComposerSecretAdapter{store: store}, nil
		},
		NewLeftoverBackupDestroyer: func(ctx context.Context, planned platform.PlannedStack) (internalcli.LeftoverBackupDestroyer, error) {
			if _, ok := providerhost.AsShimPlanned(planned); !ok {
				return nil, fmt.Errorf("leftover Cloud SQL backup destroy requires a GCP planned stack")
			}
			return gcpLeftoverBackupDestroyer{dialer: dialer, planned: planned}, nil
		},
		NewGCPCleanupProvider: newGCPCleanupProvider(dialer),
		NewCleanupProvider:    newFirstPartyCleanupProvider(dialer),
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

type gcpLeftoverBackupDestroyer struct {
	dialer  *providerhost.CachedDialer
	planned platform.PlannedStack
}

func (d gcpLeftoverBackupDestroyer) Destroy(ctx context.Context) ([]string, error) {
	client, err := d.dialer.Do(ctx)
	if err != nil {
		return nil, err
	}
	return providerhost.DestroyLeftoverBackups(ctx, client, d.planned)
}

type gcpCleanupProvider struct {
	dialer  *providerhost.CachedDialer
	project string
	marker  string
}

func (p gcpCleanupProvider) Inventory(ctx context.Context, request sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error) {
	client, err := p.dialer.Do(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Project) == "" {
		request.Project = p.project
	}
	if strings.TrimSpace(request.Marker) == "" {
		request.Marker = p.marker
	}
	return providerhost.CleanupInventory(ctx, client, request)
}

func (p gcpCleanupProvider) Delete(ctx context.Context, resource sdk.CleanupResource) error {
	client, err := p.dialer.Do(ctx)
	if err != nil {
		return err
	}
	return providerhost.CleanupDelete(ctx, client, p.project, p.marker, resource)
}

func newGCPCleanupProvider(dialer *providerhost.CachedDialer) func(context.Context, sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
	return func(_ context.Context, ledger sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
		if strings.TrimSpace(ledger.Project) == "" {
			return nil, fmt.Errorf("GCP cleanup ledgers require a project")
		}
		return gcpCleanupProvider{dialer: dialer, project: strings.TrimSpace(ledger.Project), marker: ledger.Marker}, nil
	}
}

func newFirstPartyCleanupProvider(dialer *providerhost.CachedDialer) func(context.Context, sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
	gcp := newGCPCleanupProvider(dialer)
	return func(ctx context.Context, ledger sdk.CleanupLedger) (internalcli.CleanupProvider, error) {
		switch ledger.Provider {
		case "gcp":
			return gcp(ctx, ledger)
		case "ovh":
			return newOVHDatabaseCleanupProvider(ctx, ledger)
		case "scaleway":
			return newScalewayCleanupProvider(ctx, ledger)
		default:
			return nil, cleanup.UnsupportedProvider(ledger.Provider)
		}
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
