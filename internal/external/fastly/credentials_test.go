package fastly

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakeTokenAPI struct {
	current      NativeToken
	created      NativeToken
	currentCalls int
	createCalls  int
	revokeCalls  []string
}

func (api *fakeTokenAPI) CurrentToken(context.Context) (NativeToken, error) {
	api.currentCalls++
	return api.current, nil
}

func (api *fakeTokenAPI) CreateToken(context.Context, string, string, []string) (NativeToken, error) {
	api.createCalls++
	return api.created, nil
}

func (api *fakeTokenAPI) RevokeToken(_ context.Context, id string) error {
	api.revokeCalls = append(api.revokeCalls, id)
	return nil
}

type fastlyCredentialSink struct {
	reference string
	value     []byte
}

func (sink *fastlyCredentialSink) Store(_ context.Context, reference string, value []byte) error {
	sink.reference = reference
	sink.value = append([]byte(nil), value...)
	return nil
}

func TestCredentialLifecycleRotatesAndRevokesScopedFastlyToken(t *testing.T) {
	api := &fakeTokenAPI{
		current: NativeToken{ID: "current-token", Scope: "global"},
		created: NativeToken{ID: "replacement-token", AccessToken: "replacement-secret", Scope: "global", Services: []string{"svc-1"}},
	}
	sink := &fastlyCredentialSink{}
	client, err := NewCredentialLifecycleClient(api, sink, "global", []string{"svc-1"})
	if err != nil {
		t.Fatal(err)
	}
	base := sdk.CredentialLifecycleRequest{
		Provider:                "fastly",
		CredentialRef:           "aws-secrets-manager://magelift/fastly-token",
		ManagementCredentialRef: "fastly://management",
		OwnershipMarker:         "magelift/test/fastly-credentials",
		IdempotencyKey:          "fastly-credential-1",
	}
	validated, err := sdk.RunCredentialLifecycle(context.Background(), client, sdk.CredentialLifecycleRequest{Action: sdk.CredentialValidate, Provider: base.Provider, CredentialRef: base.CredentialRef, ManagementCredentialRef: base.ManagementCredentialRef, OwnershipMarker: base.OwnershipMarker, IdempotencyKey: base.IdempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	if validated.CredentialVersion != "current-token" || api.currentCalls != 1 {
		t.Fatalf("validation result/calls = %#v/%d", validated, api.currentCalls)
	}

	rotatedRequest := base
	rotatedRequest.Action = sdk.CredentialRotate
	rotated, err := sdk.RunCredentialLifecycle(context.Background(), client, rotatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.CredentialVersion != "replacement-token" || sink.reference != base.CredentialRef || string(sink.value) != "replacement-secret" || api.createCalls != 1 {
		t.Fatalf("rotation result/sink/calls = %#v/%#v/%d", rotated, sink, api.createCalls)
	}
	if strings.Contains(rotated.OperationID, "replacement-secret") {
		t.Fatal("Fastly replacement secret escaped the lifecycle identity")
	}

	revokedRequest := base
	revokedRequest.Action = sdk.CredentialRevoke
	revokedRequest.CredentialVersion = rotated.CredentialVersion
	revoked, err := sdk.RunCredentialLifecycle(context.Background(), client, revokedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.CredentialVersion != "replacement-token" || len(api.revokeCalls) != 1 || api.revokeCalls[0] != "replacement-token" {
		t.Fatalf("revocation result/calls = %#v/%#v", revoked, api.revokeCalls)
	}
}

func TestCredentialLifecycleRejectsUnexpectedFastlyTokenScope(t *testing.T) {
	api := &fakeTokenAPI{created: NativeToken{ID: "token-1", AccessToken: "secret", Scope: "global"}}
	client, err := NewCredentialLifecycleClient(api, &fastlyCredentialSink{}, "global:read", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ExecuteCredentialLifecycle(context.Background(), sdk.CredentialLifecycleRequest{
		Action: sdk.CredentialRotate, Provider: "fastly", CredentialRef: "fastly://target",
		OwnershipMarker: "magelift/test/scope", IdempotencyKey: "rotate-1",
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected scope") {
		t.Fatalf("scope error = %v", err)
	}
	if len(api.revokeCalls) != 1 || api.revokeCalls[0] != "token-1" {
		t.Fatalf("failed rotation was not revoked: %#v", api.revokeCalls)
	}
}
