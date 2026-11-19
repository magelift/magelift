package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	awsendpoint "github.com/magelift/magelift/internal/awsendpoint"
	"github.com/magelift/magelift/internal/cleanup"
	internalcli "github.com/magelift/magelift/internal/cli"
	ovhresilience "github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// RegisterHooks constructs cloud clients lazily. Each plugin session is shared
// for the lifetime of a CLI invocation.
func RegisterHooks() internalcli.Hooks {
	return registerHooks(newProviderDialer(string(providerhost.StandaloneProviderGCP)), newProviderDialer(string(providerhost.StandaloneProviderAWS)))
}

func registerHooks(gcpDialer, awsDialer *providerhost.CachedDialer) internalcli.Hooks {
	return internalcli.Hooks{
		Close: func() {
			gcpDialer.Close()
			awsDialer.Close()
		},
		NewComposerSecrets: func(ctx context.Context, region string) (internalcli.ComposerSecretProvider, error) {
			return awsComposerSecretAdapter{dialer: awsDialer, envelope: sdk.Envelope{Region: region}}, nil
		},
		NewComposerGCPSecrets: func(ctx context.Context) (internalcli.ComposerSecretProvider, error) {
			client, err := gcpDialer.Do(ctx)
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
			return gcpLeftoverBackupDestroyer{dialer: gcpDialer, planned: planned}, nil
		},
		NewGCPCleanupProvider: newGCPCleanupProvider(gcpDialer),
		NewCleanupProvider:    newFirstPartyCleanupProvider(gcpDialer),
		MediaEndpoint: func() (string, error) {
			return awsendpoint.FromEnv()
		},
	}
}

type awsComposerSecretAdapter struct {
	dialer   *providerhost.CachedDialer
	envelope sdk.Envelope
}

func (a awsComposerSecretAdapter) GetSecretValue(ctx context.Context, name string) ([]byte, error) {
	return a.read(ctx, name, false)
}

func (a awsComposerSecretAdapter) GetParameter(ctx context.Context, name string) ([]byte, error) {
	return a.read(ctx, name, true)
}

func (a awsComposerSecretAdapter) read(ctx context.Context, name string, parameter bool) ([]byte, error) {
	client, err := a.dialer.Do(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := providerhost.Call[sdk.SecretReadRequest, sdk.SecretReadResult](ctx, client, sdk.OpSecretRead, &sdk.SecretReadRequest{
		ProtocolVersion: sdk.ProtocolV1,
		Envelope:        a.envelope,
		Name:            name,
		Parameter:       parameter,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, providerhost.AsPluginError(sdk.OpSecretRead, resp.Error)
	}
	return resp.Value, nil
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
		case string(providerhost.StandaloneProviderGCP):
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
