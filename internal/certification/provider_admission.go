package certification

import (
	"context"
	"errors"
	"fmt"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// AdmissionFact is a redacted provider-side result for one prerequisite.
type AdmissionFact struct {
	Ready  bool
	Reason string
}

// AdmissionFactChecker verifies one owning boundary such as a network,
// state backend, scrubbed fixture, or ownership marker.
type AdmissionFactChecker interface {
	Check(context.Context, AdmissionInput) (AdmissionFact, error)
}

// AdmissionQuotaChecker returns live provider quota values. It must query the
// provider's owning quota service rather than infer availability from the
// requested resource count.
type AdmissionQuotaChecker interface {
	Available(context.Context, AdmissionInput) (map[string]int, error)
}

// AdmissionLocalResourceChecker reports the local runner capacity available
// to the certification cell. It is separate from provider quota because CPU
// and memory are properties of the runner process, not a cloud account.
type AdmissionLocalResourceChecker interface {
	Available(context.Context, AdmissionInput) (cpuMilli int64, memoryMB int64, err error)
}

// ProviderAdmissionProbe composes provider identity, secret-reference,
// network, state, fixture, ownership, and quota checks without moving any
// provider SDK type into the certification core.
type ProviderAdmissionProbe struct {
	Identity            provider.IdentityClient
	RequiredPermissions []string
	Credentials         provider.CredentialResolver
	Network             AdmissionFactChecker
	StateBackend        AdmissionFactChecker
	Fixture             AdmissionFactChecker
	Ownership           AdmissionFactChecker
	Quota               AdmissionQuotaChecker
	LocalResources      AdmissionLocalResourceChecker
}

var _ AdmissionProbe = (*ProviderAdmissionProbe)(nil)

// Probe implements the live admission contract. Missing checkers produce a
// blocked fact; they are never treated as optional because an unchecked paid
// mutation is unsafe.
func (p *ProviderAdmissionProbe) Probe(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
	if p == nil {
		return AdmissionProbeResult{}, errors.New("provider admission probe is required")
	}
	if ctx == nil {
		return AdmissionProbeResult{}, errors.New("provider admission context is required")
	}
	if err := ctx.Err(); err != nil {
		return AdmissionProbeResult{}, err
	}
	result := AdmissionProbeResult{Reasons: make(map[string]string)}
	set := func(id string, ready bool, reason string) {
		if !ready {
			result.Reasons[id] = nonEmptyAdmissionReason(reason)
		}
	}

	if input.RequireCredentials {
		if p.Credentials == nil {
			result.CredentialsReady = false
			set("provider.credentials", false, "provider credential resolver is not configured")
		} else {
			result.CredentialsReady = true
			for _, reference := range input.CredentialRefs {
				err := provider.UseCredential(ctx, p.Credentials, reference, func([]byte) error { return nil })
				if err != nil {
					result.CredentialsReady = false
					set("provider.credentials", false, "provider credential reference could not be resolved")
					break
				}
			}
		}
	} else {
		result.CredentialsReady = true
	}

	if p.Identity == nil {
		set("provider.account-or-project", false, "provider identity client is not configured")
	} else {
		_, err := provider.VerifyIdentity(ctx, p.Identity, provider.IdentityCheckRequest{
			Provider:            sdk.ProviderID(input.Provider),
			AccountOrProjectRef: input.AccountOrProjectRef,
			Region:              input.Region,
			RequiredPermissions: append([]string(nil), p.RequiredPermissions...),
		})
		result.AccountOrProjectReady = err == nil
		if err != nil {
			set("provider.account-or-project", false, "provider account/project or permission scope is not verified")
		}
	}

	result.NetworkReady = p.runFact(ctx, p.Network, input, "provider.network", &result)
	result.StateBackendReady = p.runFact(ctx, p.StateBackend, input, "provider.state-backend", &result)
	result.FixtureReady = p.runFact(ctx, p.Fixture, input, "provider.fixture", &result)
	result.OwnershipReady = p.runFact(ctx, p.Ownership, input, "provider.ownership", &result)

	if len(input.RequiredQuota) == 0 {
		result.AvailableQuota = map[string]int{}
	} else if p.Quota == nil {
		set("provider.quota", false, "provider quota checker is not configured")
	} else {
		available, err := p.Quota.Available(ctx, input)
		if err != nil {
			set("provider.quota", false, "provider quota is unavailable")
		} else {
			result.AvailableQuota = cloneQuota(available)
		}
	}
	if input.RequiredLocalCPUMilli > 0 || input.RequiredLocalMemoryMB > 0 {
		if p.LocalResources == nil {
			set("provider.local-resources", false, "local resource checker is not configured")
		} else {
			cpuMilli, memoryMB, err := p.LocalResources.Available(ctx, input)
			if err != nil || cpuMilli < 0 || memoryMB < 0 {
				set("provider.local-resources", false, "local resource capacity is unavailable")
			} else {
				result.AvailableLocalCPUMilli = cpuMilli
				result.AvailableLocalMemoryMB = memoryMB
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return AdmissionProbeResult{}, err
	}
	return result, nil
}

func (p *ProviderAdmissionProbe) runFact(ctx context.Context, checker AdmissionFactChecker, input AdmissionInput, id string, result *AdmissionProbeResult) bool {
	if checker == nil {
		result.Reasons[id] = "provider admission checker is not configured"
		return false
	}
	fact, err := checker.Check(ctx, input)
	if err != nil {
		result.Reasons[id] = "provider admission check is unavailable"
		return false
	}
	if !fact.Ready {
		result.Reasons[id] = nonEmptyAdmissionReason(fact.Reason)
	}
	return fact.Ready
}

func nonEmptyAdmissionReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "provider admission requirement is not verified"
	}
	return reason
}

func cloneQuota(values map[string]int) map[string]int {
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

// StaticAdmissionFact is useful for deterministic provider-contract tests and
// for explicitly pre-verified boundaries. Live adapters should implement
// AdmissionFactChecker against their owning service instead.
type StaticAdmissionFact bool

func (f StaticAdmissionFact) Check(context.Context, AdmissionInput) (AdmissionFact, error) {
	if !f {
		return AdmissionFact{Reason: "static admission fact is false"}, nil
	}
	return AdmissionFact{Ready: true}, nil
}

// StaticAdmissionQuota is a deterministic quota adapter for local contract
// tests. It deliberately copies its map so callers cannot mutate the result
// after the probe returns.
type StaticAdmissionQuota map[string]int

func (q StaticAdmissionQuota) Available(context.Context, AdmissionInput) (map[string]int, error) {
	if q == nil {
		return nil, fmt.Errorf("static quota is unavailable")
	}
	return cloneQuota(q), nil
}

// StaticAdmissionLocalResources is a deterministic local-capacity adapter for
// contract tests and explicitly provisioned runners.
type StaticAdmissionLocalResources struct {
	CPUMilli int64
	MemoryMB int64
}

func (r StaticAdmissionLocalResources) Available(context.Context, AdmissionInput) (int64, int64, error) {
	if r.CPUMilli < 0 || r.MemoryMB < 0 {
		return 0, 0, fmt.Errorf("static local resources cannot be negative")
	}
	return r.CPUMilli, r.MemoryMB, nil
}
