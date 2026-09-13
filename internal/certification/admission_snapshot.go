package certification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

const (
	// DefaultAdmissionSnapshotSchema changes whenever the facts represented by
	// an admission result change. A new schema starts a fresh run-scoped cache.
	DefaultAdmissionSnapshotSchema = "magelift-certification-admission-v1"

	// DefaultAdmissionSnapshotMaxConcurrent keeps read-only provider probes
	// bounded when several certification schedules share one snapshot.
	DefaultAdmissionSnapshotMaxConcurrent = 4
)

// AdmissionSnapshotOptions controls a run-scoped admission cache. The cache
// contains only redacted admission facts; it never stores provider SDK values,
// credentials, or resource references.
type AdmissionSnapshotOptions struct {
	// Schema identifies the shape and semantics of the cached admission facts.
	// Changing it must start a new snapshot for the run.
	Schema string
	// MaxConcurrent bounds distinct provider probes. Zero uses the default.
	MaxConcurrent int
}

// AdmissionSnapshot memoizes read-only admission probes for one certification
// run. Provider/account/region/schema identify the reusable capability scope;
// an additional digest of the complete input prevents network, fixture, state,
// ownership, or quota facts from crossing independent evidence boundaries.
//
// The snapshot is intentionally not a process-wide cache. Callers should
// create one per certification run and discard it with the run's other state.
type AdmissionSnapshot struct {
	probe       AdmissionProbe
	schema      string
	concurrency chan struct{}

	mu       sync.Mutex
	entries  map[admissionSnapshotKey]AdmissionProbeResult
	inFlight map[admissionSnapshotKey]*admissionSnapshotCall
}

type admissionSnapshotKey struct {
	Provider            string
	AccountOrProjectRef string
	Region              string
	Schema              string
	ScopeDigest         string
}

type admissionSnapshotCall struct {
	done   chan struct{}
	result AdmissionProbeResult
	err    error
}

// NewAdmissionSnapshot creates a bounded, run-scoped memoizer around a
// provider admission probe. A zero MaxConcurrent uses the conservative default
// instead of creating an unbounded cache worker pool.
func NewAdmissionSnapshot(probe AdmissionProbe, options AdmissionSnapshotOptions) (*AdmissionSnapshot, error) {
	if isNilAdmissionProbe(probe) {
		return nil, errors.New("admission snapshot probe is required")
	}
	schema := strings.TrimSpace(options.Schema)
	if schema == "" {
		schema = DefaultAdmissionSnapshotSchema
	}
	if strings.ContainsAny(schema, "\r\n\x00") {
		return nil, errors.New("admission snapshot schema must be single-line")
	}
	if err := ValidateSecretSafeText(schema); err != nil {
		return nil, errors.New("admission snapshot schema contains secret-like material")
	}
	maxConcurrent := options.MaxConcurrent
	if maxConcurrent == 0 {
		maxConcurrent = DefaultAdmissionSnapshotMaxConcurrent
	}
	if maxConcurrent < 1 {
		return nil, errors.New("admission snapshot max concurrency must be at least one")
	}
	return &AdmissionSnapshot{
		probe:       probe,
		schema:      schema,
		concurrency: make(chan struct{}, maxConcurrent),
		entries:     make(map[admissionSnapshotKey]AdmissionProbeResult),
		inFlight:    make(map[admissionSnapshotKey]*admissionSnapshotCall),
	}, nil
}

var _ AdmissionProbe = (*AdmissionSnapshot)(nil)

// Probe returns an immutable copy of a cached result or runs exactly one
// bounded probe for a cache key. Waiting callers can cancel without interrupting
// the owner of the in-flight read-only probe.
func (snapshot *AdmissionSnapshot) Probe(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
	if snapshot == nil {
		return AdmissionProbeResult{}, errors.New("admission snapshot is required")
	}
	if ctx == nil {
		return AdmissionProbeResult{}, errors.New("admission snapshot context is required")
	}
	if err := ctx.Err(); err != nil {
		return AdmissionProbeResult{}, err
	}
	input = cloneAdmissionInput(input)
	if err := validateAdmissionInput(input); err != nil {
		return AdmissionProbeResult{}, err
	}
	key, err := snapshot.key(input)
	if err != nil {
		return AdmissionProbeResult{}, err
	}

	snapshot.mu.Lock()
	if result, found := snapshot.entries[key]; found {
		snapshot.mu.Unlock()
		return cloneAdmissionProbeResult(result), nil
	}
	if call, found := snapshot.inFlight[key]; found {
		snapshot.mu.Unlock()
		return waitForAdmissionSnapshotCall(ctx, call)
	}
	call := &admissionSnapshotCall{done: make(chan struct{})}
	snapshot.inFlight[key] = call
	snapshot.mu.Unlock()

	result, probeErr := snapshot.runProbe(ctx, input)
	if probeErr != nil {
		// A provider may return partial data alongside an error. That data is
		// neither admitted evidence nor safe to expose or cache.
		result = AdmissionProbeResult{}
	} else {
		if err := validateAdmissionSnapshotResult(result); err != nil {
			probeErr = err
			result = AdmissionProbeResult{}
		} else {
			result = cloneAdmissionProbeResult(result)
		}
	}
	probeErr = safeAdmissionSnapshotError(probeErr)

	snapshot.mu.Lock()
	delete(snapshot.inFlight, key)
	call.result = cloneAdmissionProbeResult(result)
	call.err = probeErr
	if probeErr == nil {
		snapshot.entries[key] = call.result
	}
	close(call.done)
	snapshot.mu.Unlock()
	return cloneAdmissionProbeResult(result), probeErr
}

func (snapshot *AdmissionSnapshot) key(input AdmissionInput) (admissionSnapshotKey, error) {
	data, err := canonicalAdmissionInput(input)
	if err != nil {
		return admissionSnapshotKey{}, fmt.Errorf("admission snapshot scope: %w", err)
	}
	digest := sha256.Sum256(data)
	return admissionSnapshotKey{
		Provider:            strings.TrimSpace(input.Provider),
		AccountOrProjectRef: strings.TrimSpace(input.AccountOrProjectRef),
		Region:              strings.TrimSpace(input.Region),
		Schema:              snapshot.schema,
		ScopeDigest:         hex.EncodeToString(digest[:]),
	}, nil
}

func canonicalAdmissionInput(input AdmissionInput) ([]byte, error) {
	canonical := cloneAdmissionInput(input)
	sort.Strings(canonical.CredentialRefs)
	sort.Strings(canonical.MutationKeys)
	return json.Marshal(canonical)
}

func cloneAdmissionInput(input AdmissionInput) AdmissionInput {
	clone := input
	clone.CredentialRefs = append([]string(nil), input.CredentialRefs...)
	clone.MutationKeys = append([]string(nil), input.MutationKeys...)
	clone.RequiredQuota = cloneIntMap(input.RequiredQuota)
	clone.AvailableQuota = cloneIntMap(input.AvailableQuota)
	return clone
}

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func isNilAdmissionProbe(probe AdmissionProbe) bool {
	if probe == nil {
		return true
	}
	value := reflect.ValueOf(probe)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (snapshot *AdmissionSnapshot) runProbe(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
	select {
	case snapshot.concurrency <- struct{}{}:
	case <-ctx.Done():
		return AdmissionProbeResult{}, ctx.Err()
	}
	defer func() { <-snapshot.concurrency }()
	return snapshot.probe.Probe(ctx, input)
}

func waitForAdmissionSnapshotCall(ctx context.Context, call *admissionSnapshotCall) (AdmissionProbeResult, error) {
	select {
	case <-call.done:
		return cloneAdmissionProbeResult(call.result), call.err
	case <-ctx.Done():
		return AdmissionProbeResult{}, ctx.Err()
	}
}

func validateAdmissionSnapshotResult(result AdmissionProbeResult) error {
	if err := ValidateSecretSafeValue(result); err != nil {
		return errors.New("admission snapshot result contains secret-like material")
	}
	for key, value := range result.AvailableQuota {
		if strings.TrimSpace(key) == "" || value < 0 {
			return errors.New("admission snapshot result contains an invalid quota")
		}
	}
	if result.AvailableLocalCPUMilli < 0 || result.AvailableLocalMemoryMB < 0 {
		return errors.New("admission snapshot result contains negative local capacity")
	}
	for key, reason := range result.Reasons {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key+reason, "\r\n\x00") {
			return errors.New("admission snapshot result contains an invalid reason")
		}
	}
	return nil
}

func safeAdmissionSnapshotError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("admission snapshot probe failed")
}

func cloneAdmissionProbeResult(result AdmissionProbeResult) AdmissionProbeResult {
	clone := result
	if result.AvailableQuota != nil {
		clone.AvailableQuota = make(map[string]int, len(result.AvailableQuota))
		for key, value := range result.AvailableQuota {
			clone.AvailableQuota[key] = value
		}
	}
	if result.Reasons != nil {
		clone.Reasons = make(map[string]string, len(result.Reasons))
		for key, value := range result.Reasons {
			clone.Reasons[key] = value
		}
	}
	return clone
}
