package resilience

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/ovh/okms-sdk-go"
	"github.com/ovh/okms-sdk-go/types"
)

// ovhSecretSDK wraps the official OVHcloud OKMS Go SDK and maps its versioned
// key/value model to the provider-local SecretAPI. No OKMS model crosses into
// the shared recovery lifecycle.
type ovhSecretSDK struct {
	client *okms.Client
	okmsID uuid.UUID
}

func newOVHSecretSDK(client *okms.Client, config NativeAPIConfig) *ovhSecretSDK {
	api, err := NewSecretAPIFromClient(client, config.SecretOKMSID)
	if err != nil {
		return nil
	}
	return api.(*ovhSecretSDK)
}

// NewSecretAPIFromClient exposes only the provider-local SecretAPI boundary
// around the official OVHcloud OKMS SDK. Callers never need to handle OKMS
// response models, and secret values remain inside the provider adapter.
func NewSecretAPIFromClient(client *okms.Client, okmsID string) (SecretAPI, error) {
	if client == nil {
		return nil, errors.New("OVHcloud Secret Manager SDK client is required")
	}
	parsedID, err := uuid.Parse(strings.TrimSpace(okmsID))
	if err != nil {
		return nil, fmt.Errorf("OVHcloud Secret Manager OKMS identity is invalid: %w", err)
	}
	return &ovhSecretSDK{client: client, okmsID: parsedID}, nil
}

// NewOVHSecretClient creates a regional OKMS Secret Manager client using the
// documented Bearer-token authentication. The credential resolver callback is
// the only scope in which the token is visible; the reference and token never
// enter a plan, operation ID, checkpoint, log, or evidence record.
func NewOVHSecretClient(ctx context.Context, config NativeAPIConfig, resolver provider.CredentialResolver) (*okms.Client, error) {
	if ctx == nil {
		return nil, errors.New("OVHcloud Secret Manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.SecretCredentialRef) == "" {
		return nil, errors.New("OVHcloud Secret Manager credential reference is required")
	}
	if resolver == nil {
		return nil, errors.New("OVHcloud Secret Manager credential resolver is required")
	}
	if err := validateOVHSecretConfig(config); err != nil {
		return nil, err
	}
	endpoint, err := ovhSecretEndpoint(config.SecretEndpoint)
	if err != nil {
		return nil, err
	}
	var result *okms.Client
	err = provider.UseCredential(ctx, resolver, config.SecretCredentialRef, func(value []byte) error {
		client, clientErr := okms.NewRestAPIClient(endpoint, okms.ClientConfig{TlsCfg: &tls.Config{MinVersion: tls.VersionTLS12}})
		if clientErr != nil {
			return errors.New("create OVHcloud Secret Manager client failed")
		}
		client.WithCustomHeader("Authorization", "Bearer "+string(value))
		result = client
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateOVHSecretConfig(config NativeAPIConfig) error {
	if _, err := uuid.Parse(strings.TrimSpace(config.SecretOKMSID)); err != nil {
		return fmt.Errorf("OVHcloud Secret Manager OKMS identity is invalid: %w", err)
	}
	if _, err := ovhSecretEndpoint(config.SecretEndpoint); err != nil {
		return err
	}
	return nil
}

func (client *ovhSecretSDK) Get(ctx context.Context, path string) (SecretMetadata, error) {
	if err := client.validate(ctx, path); err != nil {
		return SecretMetadata{}, err
	}
	includeData := false
	response, err := client.client.GetSecretV2(ctx, client.okmsID, path, nil, &includeData)
	if err != nil {
		return SecretMetadata{}, err
	}
	return mapOVHSecretMetadata(response, path)
}

func (client *ovhSecretSDK) List(ctx context.Context) ([]SecretMetadata, error) {
	if err := client.validate(ctx, "list"); err != nil {
		return nil, err
	}
	secrets := make([]SecretMetadata, 0)
	iterator := client.client.ListAllSecrets(client.okmsID, nil)
	for iterator.Next(ctx) {
		response, err := iterator.Value()
		if err != nil {
			return nil, err
		}
		mapped, err := mapOVHSecretMetadata(response, "")
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, mapped)
	}
	return secrets, nil
}

func (client *ovhSecretSDK) Access(ctx context.Context, path string, version uint32) ([]byte, error) {
	if err := client.validate(ctx, path); err != nil {
		return nil, err
	}
	includeData := true
	response, err := client.client.GetSecretV2(ctx, client.okmsID, path, optionalVersion(version), &includeData)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Version == nil || response.Version.Data == nil {
		return nil, errors.New("OVHcloud Secret Manager returned no secret data")
	}
	data := *response.Version.Data
	return DecodeSecretPayload(data)
}

func (client *ovhSecretSDK) Delete(ctx context.Context, path string) error {
	if err := client.validate(ctx, path); err != nil {
		return err
	}
	return client.client.DeleteSecretV2(ctx, client.okmsID, path)
}

func (client *ovhSecretSDK) Create(ctx context.Context, path string, metadata map[string]string, value []byte) (SecretMetadata, error) {
	if err := client.validate(ctx, path); err != nil {
		return SecretMetadata{}, err
	}
	data, err := EncodeSecretPayload(value)
	if err != nil {
		return SecretMetadata{}, err
	}
	custom := types.SecretV2CustomMetadata(metadata)
	response, err := client.client.PostSecretV2(ctx, client.okmsID, types.PostSecretV2Request{
		Path: path, Metadata: &types.SecretV2MetadataShort{CustomMetadata: &custom},
		Version: types.SecretV2VersionShort{Data: &data},
	})
	if err != nil {
		return SecretMetadata{}, err
	}
	return mapOVHSecretMetadataFromCreate(response, path, metadata), nil
}

func (client *ovhSecretSDK) CreateVersion(ctx context.Context, path string, value []byte) (SecretMetadata, error) {
	if err := client.validate(ctx, path); err != nil {
		return SecretMetadata{}, err
	}
	data, err := EncodeSecretPayload(value)
	if err != nil {
		return SecretMetadata{}, err
	}
	if _, err := client.client.PostSecretVersionV2(ctx, client.okmsID, path, nil, types.PostSecretVersionV2Request{Data: &data}); err != nil {
		return SecretMetadata{}, err
	}
	return client.Get(ctx, path)
}

func (client *ovhSecretSDK) validate(ctx context.Context, path string) error {
	if client == nil || client.client == nil {
		return errors.New("OVHcloud Secret Manager SDK is not configured")
	}
	if ctx == nil {
		return errors.New("OVHcloud Secret Manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" || strings.ContainsAny(path, "\r\n\x00") {
		return errors.New("OVHcloud Secret Manager path is required")
	}
	return nil
}

func mapOVHSecretMetadata(response *types.GetSecretV2Response, fallbackPath string) (SecretMetadata, error) {
	if response == nil {
		return SecretMetadata{}, errors.New("OVHcloud Secret Manager returned an empty secret")
	}
	path := fallbackPath
	if response.Path != nil && strings.TrimSpace(*response.Path) != "" {
		path = *response.Path
	}
	if strings.TrimSpace(path) == "" || response.Metadata == nil {
		return SecretMetadata{}, errors.New("OVHcloud Secret Manager returned incomplete secret metadata")
	}
	metadata := make(map[string]string)
	if response.Metadata.CustomMetadata != nil {
		for key, value := range *response.Metadata.CustomMetadata {
			metadata[key] = value
		}
	}
	var state string
	var current uint32
	if response.Version != nil {
		state = string(response.Version.State)
		current = response.Version.Id
	}
	if current == 0 && response.Metadata.CurrentVersion != nil {
		current = *response.Metadata.CurrentVersion
	}
	return SecretMetadata{Path: path, State: state, CurrentVersion: current, CustomMetadata: metadata}, nil
}

func mapOVHSecretMetadataFromCreate(response *types.PostSecretV2Response, path string, custom map[string]string) SecretMetadata {
	metadata := make(map[string]string, len(custom))
	for key, value := range custom {
		metadata[key] = value
	}
	result := SecretMetadata{Path: path, State: "active", CustomMetadata: metadata}
	if response != nil && response.Metadata != nil && response.Metadata.CurrentVersion != nil {
		result.CurrentVersion = *response.Metadata.CurrentVersion
	}
	return result
}

func optionalVersion(version uint32) *uint32 {
	if version == 0 {
		return nil
	}
	return &version
}

func ovhSecretEndpoint(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("OVHcloud Secret Manager endpoint must be an HTTPS URL without query or fragment")
	}
	return strings.TrimRight(endpoint, "/"), nil
}

var _ SecretAPI = (*ovhSecretSDK)(nil)
