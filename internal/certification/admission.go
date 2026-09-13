package certification

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// AdmissionInput is the provider-neutral evidence required before a
// certification scheduler may admit a paid mutation. Provider adapters resolve
// account, quota, network, and state identities; the core only consumes the
// redacted references and numeric capacity facts.
type AdmissionInput struct {
	Provider               string         `json:"provider" yaml:"provider"`
	AccountOrProjectRef    string         `json:"accountOrProjectRef" yaml:"accountOrProjectRef"`
	Region                 string         `json:"region" yaml:"region"`
	NetworkRef             string         `json:"networkRef" yaml:"networkRef"`
	StateBackendRef        string         `json:"stateBackendRef" yaml:"stateBackendRef"`
	FixtureID              string         `json:"fixtureId" yaml:"fixtureId"`
	OwnershipMarker        string         `json:"ownershipMarker" yaml:"ownershipMarker"`
	CredentialRefs         []string       `json:"credentialRefs,omitempty" yaml:"credentialRefs,omitempty"`
	RequireCredentials     bool           `json:"requireCredentials" yaml:"requireCredentials"`
	RequiredQuota          map[string]int `json:"requiredQuota,omitempty" yaml:"requiredQuota,omitempty"`
	AvailableQuota         map[string]int `json:"availableQuota,omitempty" yaml:"availableQuota,omitempty"`
	RequiredLocalCPUMilli  int64          `json:"requiredLocalCpuMilli" yaml:"requiredLocalCpuMilli"`
	AvailableLocalCPUMilli int64          `json:"availableLocalCpuMilli" yaml:"availableLocalCpuMilli"`
	RequiredLocalMemoryMB  int64          `json:"requiredLocalMemoryMb" yaml:"requiredLocalMemoryMb"`
	AvailableLocalMemoryMB int64          `json:"availableLocalMemoryMb" yaml:"availableLocalMemoryMb"`
	MutationKeys           []string       `json:"mutationKeys" yaml:"mutationKeys"`
}

type AdmissionStatus string

const (
	AdmissionReady   AdmissionStatus = "ready"
	AdmissionBlocked AdmissionStatus = "blocked"
)

type AdmissionCheck struct {
	ID     string          `json:"id" yaml:"id"`
	Status AdmissionStatus `json:"status" yaml:"status"`
	Reason string          `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type AdmissionReport struct {
	Ready           bool             `json:"ready" yaml:"ready"`
	Checks          []AdmissionCheck `json:"checks" yaml:"checks"`
	BlockingReasons []string         `json:"blockingReasons,omitempty" yaml:"blockingReasons,omitempty"`
}

// AdmissionProbe is the provider-side admission boundary. It must resolve
// only opaque credential references, account/project identity, network and
// state ownership, fixture availability, and live quota facts. Raw secrets
// and provider SDK objects must not appear in its result.
type AdmissionProbe interface {
	Probe(context.Context, AdmissionInput) (AdmissionProbeResult, error)
}

// AdmissionProbeResult contains the minimum live facts required before the
// scheduler may start a paid mutation. A false fact blocks admission; it is
// never interpreted as "unknown but probably ready".
type AdmissionProbeResult struct {
	CredentialsReady       bool
	AccountOrProjectReady  bool
	NetworkReady           bool
	StateBackendReady      bool
	FixtureReady           bool
	OwnershipReady         bool
	AvailableQuota         map[string]int
	AvailableLocalCPUMilli int64
	AvailableLocalMemoryMB int64
	Reasons                map[string]string
}

var admissionReferencePattern = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://[^\s]+$`)

// CheckAdmission validates the local admission contract without contacting a
// provider. Capacity shortages are a normal blocked result; malformed input
// is an error because it indicates a caller bug or an unsafe integration.
func CheckAdmission(input AdmissionInput) (AdmissionReport, error) {
	if err := validateAdmissionInput(input); err != nil {
		return AdmissionReport{}, err
	}
	report := AdmissionReport{Ready: true}
	check := func(id string, ready bool, reason string) {
		status := AdmissionReady
		if !ready {
			status = AdmissionBlocked
			report.Ready = false
			report.BlockingReasons = append(report.BlockingReasons, reason)
		}
		report.Checks = append(report.Checks, AdmissionCheck{ID: id, Status: status, Reason: reasonIfBlocked(ready, reason)})
	}

	check("credentials", !input.RequireCredentials || len(input.CredentialRefs) > 0, "required provider credentials are not available as references")
	for _, key := range sortedMapKeys(input.RequiredQuota) {
		required := input.RequiredQuota[key]
		available := input.AvailableQuota[key]
		check("quota."+key, available >= required, fmt.Sprintf("quota %q has %d available, %d required", key, available, required))
	}
	if input.RequiredLocalCPUMilli > 0 {
		check("local.cpu", input.AvailableLocalCPUMilli >= input.RequiredLocalCPUMilli, fmt.Sprintf("local CPU has %d milli available, %d required", input.AvailableLocalCPUMilli, input.RequiredLocalCPUMilli))
	}
	if input.RequiredLocalMemoryMB > 0 {
		check("local.memory", input.AvailableLocalMemoryMB >= input.RequiredLocalMemoryMB, fmt.Sprintf("local memory has %d MiB available, %d required", input.AvailableLocalMemoryMB, input.RequiredLocalMemoryMB))
	}
	sort.Strings(report.BlockingReasons)
	return report, nil
}

// CheckAdmissionWithProbe first validates the local contract, then asks the
// selected provider to prove the live admission facts. Provider failures are
// represented as blocked admission so callers cannot retry them as a PASS;
// malformed local input remains an error.
func CheckAdmissionWithProbe(ctx context.Context, input AdmissionInput, probe AdmissionProbe) (AdmissionReport, error) {
	if ctx == nil {
		return AdmissionReport{}, errors.New("admission context is required")
	}
	if isNilAdmissionProbe(probe) {
		return AdmissionReport{}, errors.New("admission probe is required")
	}
	if err := validateAdmissionInput(input); err != nil {
		return AdmissionReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return AdmissionReport{}, err
	}
	result, err := probe.Probe(ctx, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return AdmissionReport{}, err
		}
		return blockedAdmissionReport(AdmissionReport{Ready: true}, AdmissionCheck{ID: "provider-admission", Status: AdmissionBlocked, Reason: "provider admission probe failed"}), nil
	}
	if err := validateAdmissionSnapshotResult(result); err != nil {
		return blockedAdmissionReport(AdmissionReport{Ready: true}, AdmissionCheck{ID: "provider-admission", Status: AdmissionBlocked, Reason: "provider admission result is invalid"}), nil
	}
	input.AvailableQuota = cloneAdmissionQuota(result.AvailableQuota)
	input.AvailableLocalCPUMilli = result.AvailableLocalCPUMilli
	input.AvailableLocalMemoryMB = result.AvailableLocalMemoryMB
	report, err := CheckAdmission(input)
	if err != nil {
		return AdmissionReport{}, err
	}
	check := func(id string, ready bool) {
		reason := strings.TrimSpace(result.Reasons[id])
		if reason == "" {
			reason = "provider admission requirement is not verified"
		}
		status := AdmissionReady
		if !ready {
			status = AdmissionBlocked
			report.Ready = false
			report.BlockingReasons = append(report.BlockingReasons, reason)
		}
		report.Checks = append(report.Checks, AdmissionCheck{ID: id, Status: status, Reason: reasonIfBlocked(ready, reason)})
	}
	check("provider.credentials", result.CredentialsReady || !input.RequireCredentials)
	check("provider.account-or-project", result.AccountOrProjectReady)
	check("provider.network", result.NetworkReady)
	check("provider.state-backend", result.StateBackendReady)
	check("provider.fixture", result.FixtureReady)
	check("provider.ownership", result.OwnershipReady)
	for _, key := range sortedMapKeys(input.RequiredQuota) {
		available, exists := result.AvailableQuota[key]
		if !exists {
			check("provider.quota."+key, false)
			continue
		}
		required := input.RequiredQuota[key]
		reason := fmt.Sprintf("provider quota %q has %d available, %d required", key, available, required)
		ready := available >= required
		status := AdmissionReady
		if !ready {
			status = AdmissionBlocked
			report.Ready = false
			report.BlockingReasons = append(report.BlockingReasons, reason)
		}
		report.Checks = append(report.Checks, AdmissionCheck{ID: "provider.quota." + key, Status: status, Reason: reasonIfBlocked(ready, reason)})
	}
	sort.Strings(report.BlockingReasons)
	return report, nil
}

func cloneAdmissionQuota(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func blockedAdmissionReport(report AdmissionReport, check AdmissionCheck) AdmissionReport {
	report.Ready = false
	if check.Reason == "" {
		check.Reason = "provider admission requirement is not verified"
	}
	report.Checks = append(report.Checks, check)
	report.BlockingReasons = append(report.BlockingReasons, check.Reason)
	sort.Strings(report.BlockingReasons)
	return report
}

func validateAdmissionInput(input AdmissionInput) error {
	if err := ValidateSecretSafeValue(input); err != nil {
		return errors.New("admission input contains secret-like material")
	}
	for name, value := range map[string]string{
		"provider": input.Provider, "account or project reference": input.AccountOrProjectRef,
		"region": input.Region, "network reference": input.NetworkRef,
		"state backend reference": input.StateBackendRef, "fixture ID": input.FixtureID,
		"ownership marker": input.OwnershipMarker,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("admission %s is required and must not contain line breaks", name)
		}
	}
	if !admissionReferencePattern.MatchString(input.StateBackendRef) {
		return errors.New("admission state backend reference must be URI-shaped")
	}
	for _, reference := range input.CredentialRefs {
		if !admissionReferencePattern.MatchString(reference) {
			return errors.New("admission credential references must be URI-shaped secret or identity references")
		}
		if err := sdk.ValidateCredentialReference(reference); err != nil {
			return fmt.Errorf("admission credential reference: %w", err)
		}
	}
	for name, values := range map[string][]string{"credential references": input.CredentialRefs, "mutation keys": input.MutationKeys} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("admission %s must not contain empty or multiline values", name)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("admission %s contain duplicate value %q", name, value)
			}
			seen[value] = struct{}{}
		}
	}
	if len(input.MutationKeys) == 0 {
		return errors.New("admission requires at least one mutation key")
	}
	for name, values := range map[string]map[string]int{"required quota": input.RequiredQuota, "available quota": input.AvailableQuota} {
		for key, value := range values {
			if strings.TrimSpace(key) == "" || value < 0 {
				return fmt.Errorf("admission %s contains an empty key or negative value", name)
			}
		}
	}
	for name, value := range map[string]int64{
		"required local CPU": input.RequiredLocalCPUMilli, "available local CPU": input.AvailableLocalCPUMilli,
		"required local memory": input.RequiredLocalMemoryMB, "available local memory": input.AvailableLocalMemoryMB,
	} {
		if value < 0 {
			return fmt.Errorf("admission %s cannot be negative", name)
		}
	}
	return nil
}

func reasonIfBlocked(ready bool, reason string) string {
	if ready {
		return ""
	}
	return reason
}

func sortedMapKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
