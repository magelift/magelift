// Package provider contains the small internal seams used to construct
// provider-owned lifecycle clients. It deliberately contains no cloud SDK
// types; concrete providers translate these requests at their package edge.
package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// ClientRequest is safe to persist and log. CredentialRefs are opaque
// references only; a provider must resolve their values inside its own
// package and must never put the values in a plan or a returned error.
type ClientRequest struct {
	TargetID            sdk.TargetID
	Provider            sdk.ProviderID
	Runtime             sdk.RuntimeID
	AccountOrProjectRef string
	Region              string
	CredentialRefs      []string
	OwnershipMarker     string
}

// Validate checks the common construction invariants before a provider SDK
// client is created. It is intentionally independent from provider-specific
// account, region, and permission validation.
func (r ClientRequest) Validate() error {
	var problems []error
	if err := sdk.ValidateTargetDescriptor(sdk.TargetDescriptor{ID: r.TargetID, Provider: r.Provider, Runtime: r.Runtime}); err != nil {
		problems = append(problems, err)
	}
	for name, value := range map[string]string{
		"account or project reference": r.AccountOrProjectRef,
		"region":                       r.Region,
		"ownership marker":             r.OwnershipMarker,
	} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Errorf("provider client %s is required", name))
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("provider client %s must be single-line", name))
		}
	}
	seen := make(map[string]struct{}, len(r.CredentialRefs))
	for _, reference := range r.CredentialRefs {
		if err := sdk.ValidateCredentialReference(reference); err != nil {
			problems = append(problems, fmt.Errorf("provider client credential reference: %w", err))
		}
		if _, exists := seen[reference]; exists {
			problems = append(problems, fmt.Errorf("provider client credential reference %q is duplicated", reference))
		}
		seen[reference] = struct{}{}
	}
	return errors.Join(problems...)
}

// ResilienceClientFactory constructs a provider-owned client behind the
// provider-neutral resilience port. Construction must not mutate provider
// state; execution happens later through sdk.ResilienceOperationClient.
type ResilienceClientFactory interface {
	NewResilienceClient(context.Context, ClientRequest) (sdk.ResilienceOperationClient, error)
}

// EdgeClientFactory constructs a provider-owned edge adapter. The returned
// adapter is still executed by the shared SDK lifecycle gate.
type EdgeClientFactory interface {
	NewEdgeClient(context.Context, ClientRequest) (sdk.EdgeAdapter, error)
}

// ObservabilityClientFactory constructs a provider-owned observability
// adapter. Native and external destinations use the same seam, while their
// provider identities remain independent in the public plan.
type ObservabilityClientFactory interface {
	NewObservabilityClient(context.Context, ClientRequest) (sdk.ObservabilityAdapter, error)
}

// CredentialLifecycleClientFactory constructs a provider-owned credential
// lifecycle adapter. Secret values never cross this interface.
type CredentialLifecycleClientFactory interface {
	NewCredentialLifecycleClient(context.Context, ClientRequest) (sdk.CredentialLifecycleAdapter, error)
}

// InventoryClientFactory constructs an owning-service inventory client. It is
// kept separate from lifecycle clients so cleanup truth cannot be silently
// substituted by a cached plan or an adapter-local resource list.
type InventoryClientFactory interface {
	NewInventoryClient(context.Context, ClientRequest) (InventoryClient, error)
}

// InventoryResource is the normalized result of an owning-service inventory
// query. A provider must not use a secondary index or a client-side cache as
// cleanup truth.
type InventoryResource struct {
	Identity string
	Owned    bool
	Live     bool
}

// InventoryClient is shared by edge, observability, and provider lifecycle
// implementations that need an explicit owning-service cleanup check.
type InventoryClient interface {
	Inventory(context.Context, string) ([]InventoryResource, error)
}
