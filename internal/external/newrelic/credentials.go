package newrelic

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

const defaultNerdGraphEndpoint = "https://api.newrelic.com/graphql"

// CredentialSink is kept as a package alias so New Relic callers can depend
// on the provider-neutral credential boundary without importing its internal
// package in their own adapter implementation.
type CredentialSink = provider.CredentialSink

// CredentialLifecycleClient manages additional New Relic ingest-license keys
// through NerdGraph. The management credential is deliberately separate from
// the ingest credential destination: replacing an ingest key must not replace
// the user key that is still needed to create or revoke the next key.
type CredentialLifecycleClient struct {
	httpClient HTTPDoer
	resolver   provider.CredentialResolver
	sink       CredentialSink
	endpoint   string
	accountID  int64
}

var _ sdk.CredentialLifecycleAdapter = (*CredentialLifecycleClient)(nil)

// NewCredentialLifecycleClient constructs the provider-owned NerdGraph
// adapter. accountID is the New Relic account that receives generated license
// keys; it is not a secret and is never inferred from a credential value.
func NewCredentialLifecycleClient(httpClient HTTPDoer, resolver provider.CredentialResolver, sink CredentialSink, endpoint string, accountID int64) (*CredentialLifecycleClient, error) {
	if httpClient == nil {
		return nil, errors.New("New Relic NerdGraph HTTP client is required")
	}
	if resolver == nil {
		return nil, errors.New("New Relic NerdGraph credential resolver is required")
	}
	if sink == nil {
		return nil, errors.New("New Relic credential sink is required")
	}
	if accountID <= 0 || accountID > (1<<31)-1 {
		return nil, errors.New("New Relic account ID must fit the NerdGraph account ID range")
	}
	normalized, err := normalizeNerdGraphEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	return &CredentialLifecycleClient{httpClient: httpClient, resolver: resolver, sink: sink, endpoint: normalized, accountID: accountID}, nil
}

func (client *CredentialLifecycleClient) CredentialLifecycleDescriptor() sdk.CredentialLifecycleDescriptor {
	return sdk.CredentialLifecycleDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "newrelic.credentials",
		Provider:   sdk.ProviderID("newrelic"),
		Version:    "1.0.0",
		Actions:    []sdk.CredentialLifecycleAction{sdk.CredentialValidate, sdk.CredentialRotate, sdk.CredentialRevoke},
	}
}

func (client *CredentialLifecycleClient) ExecuteCredentialLifecycle(ctx context.Context, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	if ctx == nil {
		return sdk.CredentialLifecycleResult{}, errors.New("New Relic credential lifecycle context is required")
	}
	if client == nil || client.httpClient == nil || client.resolver == nil || client.sink == nil {
		return sdk.CredentialLifecycleResult{}, errors.New("New Relic credential lifecycle client is required")
	}
	if request.Provider != sdk.ProviderID("newrelic") {
		return sdk.CredentialLifecycleResult{}, errors.New("New Relic credential lifecycle provider does not match the adapter")
	}
	if err := sdk.ValidateCredentialLifecycleRequest(request); err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	managementRef := request.ManagementCredentialRef
	if managementRef == "" {
		managementRef = request.CredentialRef
	}
	switch request.Action {
	case sdk.CredentialValidate:
		return client.validate(ctx, managementRef, request)
	case sdk.CredentialRotate:
		return client.rotate(ctx, managementRef, request)
	case sdk.CredentialRevoke:
		return client.revoke(ctx, managementRef, request)
	default:
		return sdk.CredentialLifecycleResult{}, fmt.Errorf("unsupported New Relic credential lifecycle action %q", request.Action)
	}
}

func (client *CredentialLifecycleClient) validate(ctx context.Context, managementRef string, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	var response struct {
		RequestContext struct {
			UserID string `json:"userId"`
		} `json:"requestContext"`
	}
	err := client.withManagementCredential(ctx, managementRef, func(apiKey []byte) error {
		return client.graphQL(ctx, apiKey, `{ requestContext { userId } }`, nil, &response)
	})
	if err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	if strings.TrimSpace(response.RequestContext.UserID) == "" {
		return sdk.CredentialLifecycleResult{}, errors.New("New Relic NerdGraph credential validation returned no principal")
	}
	return sdk.CredentialLifecycleResult{Action: request.Action, OperationID: "newrelic:credential:validate", ScopeVerified: true, RedactionVerified: true}, nil
}

func (client *CredentialLifecycleClient) rotate(ctx context.Context, managementRef string, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	name := "magelift/" + request.OwnershipMarker
	notes := "MageLift-managed ingest license key; ownership marker=" + request.OwnershipMarker
	query := `mutation($accountId: Int!, $name: String!, $notes: String!) {
		apiAccessCreateKeys(keys: {ingest: {accountId: $accountId, ingestType: LICENSE, name: $name, notes: $notes}}) {
			createdKeys { id key type }
			errors { message type }
		}
	}`
	variables := map[string]any{"accountId": client.accountID, "name": name, "notes": notes}
	var response struct {
		Create struct {
			CreatedKeys []struct {
				ID   string `json:"id"`
				Key  string `json:"key"`
				Type string `json:"type"`
			} `json:"createdKeys"`
			Errors []graphQLError `json:"errors"`
		} `json:"apiAccessCreateKeys"`
	}
	var createdID string
	var createdKey string
	err := client.withManagementCredential(ctx, managementRef, func(apiKey []byte) error {
		if err := client.graphQL(ctx, apiKey, query, variables, &response); err != nil {
			return err
		}
		if len(response.Create.Errors) > 0 {
			return errors.New("New Relic NerdGraph key creation was rejected")
		}
		if len(response.Create.CreatedKeys) != 1 {
			return errors.New("New Relic NerdGraph key creation returned an invalid result")
		}
		createdID = response.Create.CreatedKeys[0].ID
		createdKey = response.Create.CreatedKeys[0].Key
		if err := validateProviderIdentity("New Relic created key ID", createdID); err != nil {
			return err
		}
		if strings.TrimSpace(createdKey) == "" {
			return errors.New("New Relic NerdGraph key creation returned no secret")
		}
		secret := []byte(createdKey)
		defer clear(secret)
		if err := client.sink.Store(ctx, request.CredentialRef, secret); err != nil {
			// A created key that cannot be stored is unsafe to leave behind. Best
			// effort cleanup happens while the management key is still scoped.
			_ = client.deleteIngestKey(ctx, apiKey, createdID)
			return errors.New("store rotated New Relic credential failed")
		}
		return nil
	})
	if err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	return sdk.CredentialLifecycleResult{
		Action: request.Action, OperationID: "newrelic:credential:rotate:" + createdID, CredentialVersion: createdID,
		ScopeVerified: true, RedactionVerified: true,
	}, nil
}

func (client *CredentialLifecycleClient) revoke(ctx context.Context, managementRef string, request sdk.CredentialLifecycleRequest) (sdk.CredentialLifecycleResult, error) {
	if err := validateProviderIdentity("New Relic credential version", request.CredentialVersion); err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	var deletedID string
	err := client.withManagementCredential(ctx, managementRef, func(apiKey []byte) error {
		deletedID = request.CredentialVersion
		return client.deleteIngestKey(ctx, apiKey, deletedID)
	})
	if err != nil {
		return sdk.CredentialLifecycleResult{}, err
	}
	return sdk.CredentialLifecycleResult{Action: request.Action, OperationID: "newrelic:credential:revoke:" + deletedID, CredentialVersion: deletedID, ScopeVerified: true, RedactionVerified: true}, nil
}

func (client *CredentialLifecycleClient) deleteIngestKey(ctx context.Context, apiKey []byte, keyID string) error {
	query := `mutation {
		apiAccessDeleteKeys(keys: {ingestKeyIds: ` + strconv.Quote(keyID) + `}) {
			deletedKeys { id }
			errors { message type }
		}
	}`
	var response struct {
		Delete struct {
			DeletedKeys []struct {
				ID string `json:"id"`
			} `json:"deletedKeys"`
			Errors []graphQLError `json:"errors"`
		} `json:"apiAccessDeleteKeys"`
	}
	if err := client.graphQL(ctx, apiKey, query, nil, &response); err != nil {
		return err
	}
	if len(response.Delete.Errors) > 0 || len(response.Delete.DeletedKeys) != 1 || response.Delete.DeletedKeys[0].ID != keyID {
		return errors.New("New Relic NerdGraph key revocation was not confirmed")
	}
	return nil
}

func (client *CredentialLifecycleClient) withManagementCredential(ctx context.Context, reference string, consume func([]byte) error) error {
	return provider.UseCredential(ctx, client.resolver, reference, consume)
}

func (client *CredentialLifecycleClient) graphQL(ctx context.Context, apiKey []byte, query string, variables map[string]any, output any) error {
	return executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, variables, output)
}

func normalizeNerdGraphEndpoint(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultNerdGraphEndpoint
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("New Relic NerdGraph endpoint must be an HTTPS URL without credentials or query parameters")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateProviderIdentity(name, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") || strings.ContainsAny(value, " \t") {
		return fmt.Errorf("%s must be a non-empty single-line identity", name)
	}
	return nil
}
