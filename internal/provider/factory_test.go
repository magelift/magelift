package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

type resilienceFactoryFunc func(context.Context, ClientRequest) (sdk.ResilienceOperationClient, error)

func (f resilienceFactoryFunc) NewResilienceClient(ctx context.Context, request ClientRequest) (sdk.ResilienceOperationClient, error) {
	return f(ctx, request)
}

type edgeFactoryFunc func(context.Context, ClientRequest) (sdk.EdgeAdapter, error)

func (f edgeFactoryFunc) NewEdgeClient(ctx context.Context, request ClientRequest) (sdk.EdgeAdapter, error) {
	return f(ctx, request)
}

type observabilityFactoryFunc func(context.Context, ClientRequest) (sdk.ObservabilityAdapter, error)

func (f observabilityFactoryFunc) NewObservabilityClient(ctx context.Context, request ClientRequest) (sdk.ObservabilityAdapter, error) {
	return f(ctx, request)
}

type credentialFactoryFunc func(context.Context, ClientRequest) (sdk.CredentialLifecycleAdapter, error)

func (f credentialFactoryFunc) NewCredentialLifecycleClient(ctx context.Context, request ClientRequest) (sdk.CredentialLifecycleAdapter, error) {
	return f(ctx, request)
}

type inventoryFactoryFunc func(context.Context, ClientRequest) (InventoryClient, error)

func (f inventoryFactoryFunc) NewInventoryClient(ctx context.Context, request ClientRequest) (InventoryClient, error) {
	return f(ctx, request)
}

type inventoryFunc func(context.Context, string) ([]InventoryResource, error)

func (f inventoryFunc) Inventory(ctx context.Context, marker string) ([]InventoryResource, error) {
	return f(ctx, marker)
}

func TestConstructorsValidateBeforeCallingProviderFactories(t *testing.T) {
	request := validClientRequest()
	called := false
	factory := resilienceFactoryFunc(func(context.Context, ClientRequest) (sdk.ResilienceOperationClient, error) {
		called = true
		return nil, nil
	})
	invalid := request
	invalid.TargetID = ""
	if _, err := ConstructResilienceClient(context.Background(), factory, invalid); err == nil || called {
		t.Fatalf("invalid provider request = %v, factory called = %v", err, called)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ConstructResilienceClient(ctx, factory, request); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled construction = %v, factory called = %v", err, called)
	}
}

func TestConstructorsCoverEveryLifecycleFamilyAndRedactFactoryErrors(t *testing.T) {
	request := validClientRequest()
	secret := "token=super-secret-value"
	if _, err := ConstructResilienceClient(context.Background(), resilienceFactoryFunc(func(context.Context, ClientRequest) (sdk.ResilienceOperationClient, error) {
		return nil, errors.New(secret)
	}), request); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("resilience construction error = %v", err)
	}
	if _, err := ConstructEdgeClient(context.Background(), edgeFactoryFunc(func(context.Context, ClientRequest) (sdk.EdgeAdapter, error) { return nil, nil }), request); err == nil {
		t.Fatal("nil edge client was accepted")
	}
	if _, err := ConstructObservabilityClient(context.Background(), observabilityFactoryFunc(func(context.Context, ClientRequest) (sdk.ObservabilityAdapter, error) { return nil, nil }), request); err == nil {
		t.Fatal("nil observability client was accepted")
	}
	if _, err := ConstructCredentialLifecycleClient(context.Background(), credentialFactoryFunc(func(context.Context, ClientRequest) (sdk.CredentialLifecycleAdapter, error) { return nil, nil }), request); err == nil {
		t.Fatal("nil credential lifecycle client was accepted")
	}
	if _, err := ConstructInventoryClient(context.Background(), inventoryFactoryFunc(func(context.Context, ClientRequest) (InventoryClient, error) { return nil, nil }), request); err == nil {
		t.Fatal("nil inventory client was accepted")
	}
	if _, err := ConstructEdgeClient(context.Background(), nil, request); err == nil || !strings.Contains(err.Error(), "factory") {
		t.Fatalf("nil edge factory error = %v", err)
	}
}

func TestConstructInventoryClientReturnsOwningServiceClient(t *testing.T) {
	request := validClientRequest()
	client, err := ConstructInventoryClient(context.Background(), inventoryFactoryFunc(func(_ context.Context, got ClientRequest) (InventoryClient, error) {
		if got.OwnershipMarker != request.OwnershipMarker {
			t.Fatalf("request = %#v", got)
		}
		return inventoryFunc(func(context.Context, string) ([]InventoryResource, error) {
			return []InventoryResource{{Identity: "owned", Owned: true, Live: true}}, nil
		}), nil
	}), request)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := client.Inventory(context.Background(), request.OwnershipMarker)
	if err != nil || len(resources) != 1 || resources[0].Identity != "owned" {
		t.Fatalf("inventory = %#v, err = %v", resources, err)
	}
}
