package fastly

import (
	"context"
	"errors"
	"fmt"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// CredentialLifecycleClient manages Fastly API tokens through an injected
// official-SDK-backed TokenAPI. The TokenAPI is constructed with the separate
// management credential by the provider factory; this client only receives
// opaque lifecycle requests and writes replacement tokens to a sink.
type CredentialLifecycleClient struct {
	tokens   TokenAPI
	sink     provider.CredentialSink
	scope    string
	services []string
}

var _ sdk.CredentialLifecycleAdapter = (*CredentialLifecycleClient)(nil)

func NewCredentialLifecycleClient(tokens TokenAPI, sink provider.CredentialSink, scope string, services []string) (*CredentialLifecycleClient, error) {
	if tokens == nil {
		return nil, errors.New("Fastly token API is required")
	}
	if sink == nil {
		return nil, errors.New("Fastly credential sink is required")
	}
	if strings.TrimSpace(scope) == "" || strings.ContainsAny(scope, "\r\n\x00") {
		return nil, errors.New("Fastly token scope is required and must be single-line")
	}
	seen := make(map[string]struct{}, len(services))
	for _, service := range services {
		if strings.TrimSpace(service) == "" || strings.ContainsAny(service, "\r\n\x00") {
			return nil, errors.New("Fastly token service identity is invalid")
		}
		if _, exists := seen[service]; exists {
			return nil, fmt.Errorf("duplicate Fastly token service identity %q", service)
		}
		seen[service] = struct{}{}
	}
	return &CredentialLifecycleClient{tokens: tokens, sink: sink, scope: scope, services: append([]string(nil), services...)}, nil
}

func (client *CredentialLifecycleClient) CredentialLifecycleDescriptor() sdk.CredentialLifecycleDescriptor {
	return sdk.CredentialLifecycleDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "fastly.credentials",
		Provider:   sdk.ProviderID("fastly"),
		Version:    "1.0.0",
		Actions:    []sdk.CredentialLifecycleAction{sdk.CredentialValidate, sdk.CredentialRotate, sdk.CredentialRevoke},
	}
}

func (client *CredentialLifecycleClient) ExecuteCredentialLifecycle(ctx context.Context, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	if ctx == nil {
		return sdk.CredentialLifecycleResult{}, errors.New("Fastly credential lifecycle context is required")
	}
	if client == nil || client.tokens == nil || client.sink == nil {
		return sdk.CredentialLifecycleResult{}, errors.New("Fastly credential lifecycle client is required")
	}
	if request.Provider != sdk.ProviderID("fastly") {
		return sdk.CredentialLifecycleResult{}, errors.New("Fastly credential lifecycle provider does not match the adapter")
	}
	if err := sdk.ValidateCredentialLifecycleRequest(request); err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	switch request.Action {
	case sdk.CredentialValidate:
		return client.validate(ctx, request)
	case sdk.CredentialRotate:
		return client.rotate(ctx, request)
	case sdk.CredentialRevoke:
		return client.revoke(ctx, request)
	default:
		return sdk.CredentialLifecycleResult{}, fmt.Errorf("unsupported Fastly credential lifecycle action %q", request.Action)
	}
}

func (client *CredentialLifecycleClient) validate(ctx context.Context, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	token, err := client.tokens.CurrentToken(ctx)
	if err != nil {
		return sdk.CredentialLifecycleResult{}, errors.New("validate Fastly API token failed")
	}
	if err := validateTokenIdentity(token.ID); err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	return sdk.CredentialLifecycleResult{Action: request.Action, OperationID: "fastly:credential:validate:" + token.ID, CredentialVersion: token.ID, ScopeVerified: true, RedactionVerified: true}, nil
}

func (client *CredentialLifecycleClient) rotate(ctx context.Context, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	token, err := client.tokens.CreateToken(ctx, "magelift/"+request.OwnershipMarker, client.scope, client.services)
	if err != nil {
		return sdk.CredentialLifecycleResult{}, errors.New("create Fastly API token failed")
	}
	if err := validateTokenIdentity(token.ID); err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		_ = client.tokens.RevokeToken(ctx, token.ID)
		return sdk.CredentialLifecycleResult{}, errors.New("Fastly token creation returned no secret")
	}
	if token.Scope != client.scope {
		_ = client.tokens.RevokeToken(ctx, token.ID)
		return sdk.CredentialLifecycleResult{}, errors.New("Fastly token creation returned an unexpected scope")
	}
	secret := []byte(token.AccessToken)
	defer clear(secret)
	if err := client.sink.Store(ctx, request.CredentialRef, secret); err != nil {
		_ = client.tokens.RevokeToken(ctx, token.ID)
		return sdk.CredentialLifecycleResult{}, errors.New("store rotated Fastly credential failed")
	}
	return sdk.CredentialLifecycleResult{Action: request.Action, OperationID: "fastly:credential:rotate:" + token.ID, CredentialVersion: token.ID, ScopeVerified: true, RedactionVerified: true}, nil
}

func (client *CredentialLifecycleClient) revoke(ctx context.Context, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	if err := validateTokenIdentity(request.CredentialVersion); err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	if err := client.tokens.RevokeToken(ctx, request.CredentialVersion); err != nil {
		return sdk.CredentialLifecycleResult{}, errors.New("revoke Fastly API token failed")
	}
	return sdk.CredentialLifecycleResult{Action: request.Action, OperationID: "fastly:credential:revoke:" + request.CredentialVersion, CredentialVersion: request.CredentialVersion, ScopeVerified: true, RedactionVerified: true}, nil
}

func validateTokenIdentity(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") || strings.ContainsAny(value, " \t") {
		return errors.New("Fastly token identity must be a non-empty single-line value")
	}
	return nil
}
