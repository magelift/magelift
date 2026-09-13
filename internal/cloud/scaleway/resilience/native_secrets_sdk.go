package resilience

import (
	"context"
	"errors"
	"os"
	"strings"

	secret "github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// SecretSDKAPI is the official Scaleway Secret Manager API surface consumed
// by the provider translator. Keeping generated SDK types behind this seam
// makes the adapter deterministic to test without moving them into plans or
// the platform contract.
type SecretSDKAPI interface {
	GetSecret(*secret.GetSecretRequest, ...scw.RequestOption) (*secret.Secret, error)
	ListSecrets(*secret.ListSecretsRequest, ...scw.RequestOption) (*secret.ListSecretsResponse, error)
	AccessSecretVersion(*secret.AccessSecretVersionRequest, ...scw.RequestOption) (*secret.AccessSecretVersionResponse, error)
	DeleteSecret(*secret.DeleteSecretRequest, ...scw.RequestOption) error
	CreateSecret(*secret.CreateSecretRequest, ...scw.RequestOption) (*secret.Secret, error)
	CreateSecretVersion(*secret.CreateSecretVersionRequest, ...scw.RequestOption) (*secret.SecretVersion, error)
	ProtectSecret(*secret.ProtectSecretRequest, ...scw.RequestOption) (*secret.Secret, error)
	UnprotectSecret(*secret.UnprotectSecretRequest, ...scw.RequestOption) (*secret.Secret, error)
}

type scalewaySecretSDK struct {
	api     SecretSDKAPI
	region  scw.Region
	project string
}

// NewScalewaySecretAPI constructs the provider-owned Secret Manager boundary
// from the authenticated Scaleway SDK. Credentials remain in the SDK's
// profile/environment chain and never enter a plan, result, or error.
func NewScalewaySecretAPI(ctx context.Context, project, region string, options ...scw.ClientOption) (SecretAPI, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway Secret Manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project = strings.TrimSpace(project)
	region = strings.TrimSpace(region)
	if project == "" || region == "" || strings.ContainsAny(project+region, "\r\n\x00") {
		return nil, errors.New("Scaleway Secret Manager project and region are required and must be single-line")
	}

	clientOptions, err := scalewaySecretClientOptions(project, region, options...)
	if err != nil {
		return nil, err
	}
	client, err := scw.NewClient(clientOptions...)
	if err != nil {
		return nil, errors.New("create Scaleway Secret Manager SDK client")
	}
	return NewScalewaySecretAPIFromClient(secret.NewAPI(client), project, region)
}

// NewScalewaySecretAPIFromClient injects the official SDK surface for
// deterministic tests and provider-owned implementations.
func NewScalewaySecretAPIFromClient(api SecretSDKAPI, project, region string) (SecretAPI, error) {
	project = strings.TrimSpace(project)
	region = strings.TrimSpace(region)
	if api == nil {
		return nil, errors.New("Scaleway Secret Manager SDK API is required")
	}
	if project == "" || region == "" || strings.ContainsAny(project+region, "\r\n\x00") {
		return nil, errors.New("Scaleway Secret Manager project and region are required and must be single-line")
	}
	return &scalewaySecretSDK{api: api, region: scw.Region(region), project: project}, nil
}

func scalewaySecretClientOptions(project, region string, options ...scw.ClientOption) ([]scw.ClientOption, error) {
	clientOptions := make([]scw.ClientOption, 0, len(options)+4)
	if config, err := scw.LoadConfig(); err == nil {
		profile, profileErr := config.GetActiveProfile()
		if profileErr != nil {
			return nil, errors.New("load active Scaleway SDK profile")
		}
		clientOptions = append(clientOptions, scw.WithProfile(profile))
	} else {
		var configNotFound scw.ConfigFileNotFoundError
		if !errors.As(err, &configNotFound) || os.Getenv(scw.ScwActiveProfileEnv) != "" || os.Getenv(scw.ScwConfigPathEnv) != "" {
			return nil, errors.New("load Scaleway SDK configuration")
		}
	}
	clientOptions = append(clientOptions, scw.WithEnv())
	clientOptions = append(clientOptions, options...)
	clientOptions = append(clientOptions, scw.WithDefaultProjectID(project), scw.WithDefaultRegion(scw.Region(region)))
	return clientOptions, nil
}

func (client *scalewaySecretSDK) Get(ctx context.Context, id string) (SecretMetadata, error) {
	if client == nil || client.api == nil {
		return SecretMetadata{}, errors.New("Scaleway Secret Manager SDK is not configured")
	}
	value, err := client.api.GetSecret(&secret.GetSecretRequest{Region: client.region, SecretID: id}, scw.WithContext(ctx))
	if err != nil {
		return SecretMetadata{}, err
	}
	return scalewaySecretMetadata(value)
}

func (client *scalewaySecretSDK) List(ctx context.Context) ([]SecretMetadata, error) {
	if client == nil || client.api == nil {
		return nil, errors.New("Scaleway Secret Manager SDK is not configured")
	}
	secrets := make([]SecretMetadata, 0)
	for page := int32(1); ; page++ {
		request := &secret.ListSecretsRequest{Region: client.region, Page: &page}
		if client.project != "" {
			request.ProjectID = &client.project
		}
		response, err := client.api.ListSecrets(request, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("Scaleway Secret Manager listing returned an empty response")
		}
		for _, value := range response.Secrets {
			mapped, err := scalewaySecretMetadata(value)
			if err != nil {
				return nil, err
			}
			secrets = append(secrets, mapped)
		}
		if len(response.Secrets) == 0 || uint64(len(secrets)) >= response.TotalCount {
			return secrets, nil
		}
	}
}

func (client *scalewaySecretSDK) Access(ctx context.Context, id, revision string) ([]byte, error) {
	if client == nil || client.api == nil {
		return nil, errors.New("Scaleway Secret Manager SDK is not configured")
	}
	value, err := client.api.AccessSecretVersion(&secret.AccessSecretVersionRequest{Region: client.region, SecretID: id, Revision: revision}, scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("Scaleway Secret Manager access returned an empty response")
	}
	return append([]byte(nil), value.Data...), nil
}

func (client *scalewaySecretSDK) Delete(ctx context.Context, id string) error {
	if client == nil || client.api == nil {
		return errors.New("Scaleway Secret Manager SDK is not configured")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("Scaleway Secret Manager secret ID is required")
	}
	return client.api.DeleteSecret(&secret.DeleteSecretRequest{Region: client.region, SecretID: id}, scw.WithContext(ctx))
}

func (client *scalewaySecretSDK) Create(ctx context.Context, name string, tags []string, protected bool) (SecretMetadata, error) {
	if client == nil || client.api == nil {
		return SecretMetadata{}, errors.New("Scaleway Secret Manager SDK is not configured")
	}
	value, err := client.api.CreateSecret(&secret.CreateSecretRequest{Region: client.region, ProjectID: client.project, Name: name, Tags: append([]string(nil), tags...), Protected: protected}, scw.WithContext(ctx))
	if err != nil {
		return SecretMetadata{}, err
	}
	return scalewaySecretMetadata(value)
}

func (client *scalewaySecretSDK) CreateVersion(ctx context.Context, id string, data []byte) error {
	if client == nil || client.api == nil {
		return errors.New("Scaleway Secret Manager SDK is not configured")
	}
	_, err := client.api.CreateSecretVersion(&secret.CreateSecretVersionRequest{Region: client.region, SecretID: id, Data: append([]byte(nil), data...)}, scw.WithContext(ctx))
	return err
}

func (client *scalewaySecretSDK) Protect(ctx context.Context, id string) (SecretMetadata, error) {
	if client == nil || client.api == nil {
		return SecretMetadata{}, errors.New("Scaleway Secret Manager SDK is not configured")
	}
	value, err := client.api.ProtectSecret(&secret.ProtectSecretRequest{Region: client.region, SecretID: id}, scw.WithContext(ctx))
	if err != nil {
		return SecretMetadata{}, err
	}
	return scalewaySecretMetadata(value)
}

func (client *scalewaySecretSDK) Unprotect(ctx context.Context, id string) (SecretMetadata, error) {
	if client == nil || client.api == nil {
		return SecretMetadata{}, errors.New("Scaleway Secret Manager SDK is not configured")
	}
	if strings.TrimSpace(id) == "" {
		return SecretMetadata{}, errors.New("Scaleway Secret Manager secret ID is required")
	}
	value, err := client.api.UnprotectSecret(&secret.UnprotectSecretRequest{Region: client.region, SecretID: id}, scw.WithContext(ctx))
	if err != nil {
		return SecretMetadata{}, err
	}
	return scalewaySecretMetadata(value)
}

func scalewaySecretMetadata(value *secret.Secret) (SecretMetadata, error) {
	if value == nil {
		return SecretMetadata{}, errors.New("Scaleway Secret Manager API returned an empty secret")
	}
	return SecretMetadata{ID: value.ID, Name: value.Name, Status: string(value.Status), Protected: value.Protected, Tags: append([]string(nil), value.Tags...)}, nil
}

var _ SecretAPI = (*scalewaySecretSDK)(nil)
var _ SecretSDKAPI = (*secret.API)(nil)
