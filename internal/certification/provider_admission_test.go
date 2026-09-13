package certification

import (
	"context"
	"errors"
	"strings"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
)

type identityClientFunc func(context.Context, provider.IdentityCheckRequest) (provider.IdentityObservation, error)

func (f identityClientFunc) CheckIdentity(ctx context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
	return f(ctx, request)
}

func TestProviderAdmissionProbeRequiresEveryBoundary(t *testing.T) {
	input := validAdmissionInput()
	probe := &ProviderAdmissionProbe{
		Identity: identityClientFunc(func(_ context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
			return provider.IdentityObservation{Provider: request.Provider, AccountOrProjectRef: request.AccountOrProjectRef, Region: request.Region, PrincipalRef: "principal", LeastPrivilegeVerified: true}, nil
		}),
		Credentials: provider.CredentialResolverFunc(func(_ context.Context, _ string, consume func([]byte) error) error { return consume([]byte("opaque")) }),
		Network:     StaticAdmissionFact(true), StateBackend: StaticAdmissionFact(true), Fixture: StaticAdmissionFact(true), Ownership: StaticAdmissionFact(true),
		Quota:          StaticAdmissionQuota{"vpc": 2},
		LocalResources: StaticAdmissionLocalResources{CPUMilli: 1000, MemoryMB: 1024},
	}
	result, err := probe.Probe(context.Background(), input)
	if err != nil || !result.CredentialsReady || !result.AccountOrProjectReady || !result.NetworkReady || !result.StateBackendReady || !result.FixtureReady || !result.OwnershipReady || result.AvailableQuota["vpc"] != 2 {
		t.Fatalf("provider admission result = %#v, %v", result, err)
	}

	probe.Network = nil
	result, err = probe.Probe(context.Background(), input)
	if err != nil || result.NetworkReady || !strings.Contains(result.Reasons["provider.network"], "not configured") {
		t.Fatalf("missing network proof = %#v, %v", result, err)
	}

	probe.Network = StaticAdmissionFact(true)
	probe.LocalResources = StaticAdmissionLocalResources{CPUMilli: 250, MemoryMB: 1024}
	result, err = probe.Probe(context.Background(), input)
	if err != nil || result.AvailableLocalCPUMilli != 250 || result.AvailableLocalMemoryMB != 1024 {
		t.Fatalf("local resource capacity = %#v, %v", result, err)
	}
	probe.LocalResources = nil
	result, err = probe.Probe(context.Background(), input)
	if err != nil || !strings.Contains(result.Reasons["provider.local-resources"], "not configured") {
		t.Fatalf("missing local resource proof = %#v, %v", result, err)
	}
}

func TestProviderAdmissionProbeDoesNotLeakResolverError(t *testing.T) {
	probe := &ProviderAdmissionProbe{Credentials: provider.CredentialResolverFunc(func(context.Context, string, func([]byte) error) error {
		return errors.New("token=super-secret-value")
	})}
	result, err := probe.Probe(context.Background(), validAdmissionInput())
	if err != nil || result.CredentialsReady || strings.Contains(strings.Join(mapValues(result.Reasons), ";"), "super-secret-value") {
		t.Fatalf("credential failure = %#v, %v", result, err)
	}
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

var _ provider.IdentityClient = identityClientFunc(nil)
