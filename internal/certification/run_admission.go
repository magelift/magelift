package certification

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// CertificationRunAdmissionOptions defines the complete set of admission
// scopes required by one certification run. Each input is a full
// AdmissionInput; partial provider/account/region placeholders are rejected
// before the snapshot is constructed.
type CertificationRunAdmissionOptions struct {
	// RequiredInputs are the provider/account/region and resource-boundary
	// scopes that the run must admit before it can start paid mutations.
	RequiredInputs []AdmissionInput
	// Snapshot controls the schema and maximum number of distinct provider
	// probes shared by this run.
	Snapshot AdmissionSnapshotOptions
}

// CertificationRunAdmission owns the one read-only admission snapshot for a
// certification run. It is provider-neutral: provider implementations are
// injected through AdmissionProbe, normally as a configured
// *ProviderAdmissionProbe, and no provider SDK value crosses this boundary.
//
// The value is immutable after construction and is safe to use concurrently.
// Callers should keep it for the lifetime of the run and discard it with the
// run's other state.
type CertificationRunAdmission struct {
	snapshot      *AdmissionSnapshot
	inputs        []AdmissionInput
	scopes        map[admissionSnapshotKey]struct{}
	maxConcurrent int
}

// CertificationRunAdmissionScopeReport is the redacted result for one
// required input. It identifies the scope without returning credential
// references or any other full AdmissionInput material.
type CertificationRunAdmissionScopeReport struct {
	InputIndex          int             `json:"inputIndex" yaml:"inputIndex"`
	Provider            string          `json:"provider" yaml:"provider"`
	AccountOrProjectRef string          `json:"accountOrProjectRef" yaml:"accountOrProjectRef"`
	Region              string          `json:"region" yaml:"region"`
	Admission           AdmissionReport `json:"admission" yaml:"admission"`
}

// CertificationRunAdmissionReport is the deterministic aggregate returned by
// Admit. A provider-side failure is represented as a blocked scope in the
// report; only malformed inputs, cancellation, or an internal orchestration
// failure are returned as an error.
type CertificationRunAdmissionReport struct {
	Ready  bool                                   `json:"ready" yaml:"ready"`
	Scopes []CertificationRunAdmissionScopeReport `json:"scopes" yaml:"scopes"`
}

// NewCertificationRunAdmission validates all required inputs and constructs
// exactly one run-scoped AdmissionSnapshot around the injected provider
// probe. The snapshot's concurrency limit and the orchestration worker pool
// use the same bounded value, so a large certification matrix cannot create an
// unbounded number of provider calls or goroutines.
func NewCertificationRunAdmission(probe AdmissionProbe, options CertificationRunAdmissionOptions) (*CertificationRunAdmission, error) {
	if isNilAdmissionProbe(probe) {
		return nil, errors.New("certification run admission probe is required")
	}
	if len(options.RequiredInputs) == 0 {
		return nil, errors.New("certification run admission requires at least one full admission input")
	}

	inputs := make([]AdmissionInput, len(options.RequiredInputs))
	for index, input := range options.RequiredInputs {
		input = cloneAdmissionInput(input)
		if err := validateAdmissionInput(input); err != nil {
			// Do not return the underlying validator error: it may contain an
			// untrusted reference or provider diagnostic. The index is enough to
			// identify the caller's bad matrix row without exposing its contents.
			return nil, fmt.Errorf("certification run admission input %d is invalid", index)
		}
		inputs[index] = input
	}

	workers := options.Snapshot.MaxConcurrent
	if workers == 0 {
		workers = DefaultAdmissionSnapshotMaxConcurrent
	}
	if workers < 1 {
		return nil, errors.New("certification run admission max concurrency must be at least one")
	}
	if workers > len(inputs) {
		workers = len(inputs)
	}

	snapshotOptions := options.Snapshot
	snapshotOptions.MaxConcurrent = workers
	snapshot, err := NewAdmissionSnapshot(probe, snapshotOptions)
	if err != nil {
		return nil, fmt.Errorf("certification run admission snapshot: %w", err)
	}

	scopes := make(map[admissionSnapshotKey]struct{}, len(inputs))
	for index, input := range inputs {
		key, err := snapshot.key(input)
		if err != nil {
			return nil, fmt.Errorf("certification run admission input %d is invalid", index)
		}
		scopes[key] = struct{}{}
	}

	return &CertificationRunAdmission{
		snapshot:      snapshot,
		inputs:        inputs,
		scopes:        scopes,
		maxConcurrent: workers,
	}, nil
}

var _ AdmissionProbe = (*CertificationRunAdmission)(nil)

// Snapshot returns the single run-scoped snapshot. It is intended for a live
// harness to pass to SchedulerOptions.AdmissionSnapshot when several schedule
// builds share this run. The returned snapshot has no mutating methods.
func (run *CertificationRunAdmission) Snapshot() *AdmissionSnapshot {
	if run == nil {
		return nil
	}
	return run.snapshot
}

// RequiredInputs returns defensive copies of the full scopes admitted by the
// run. CredentialRefs remain opaque references; credential values are never
// accepted by this API or returned by this method.
func (run *CertificationRunAdmission) RequiredInputs() []AdmissionInput {
	if run == nil {
		return nil
	}
	inputs := make([]AdmissionInput, len(run.inputs))
	for index, input := range run.inputs {
		inputs[index] = cloneAdmissionInput(input)
	}
	return inputs
}

// Probe admits one input through this run's snapshot. Only an input declared
// in RequiredInputs is accepted, which prevents a caller from silently
// expanding the live run after its admission boundary was established.
func (run *CertificationRunAdmission) Probe(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
	if run == nil || run.snapshot == nil {
		return AdmissionProbeResult{}, errors.New("certification run admission is required")
	}
	if ctx == nil {
		return AdmissionProbeResult{}, errors.New("certification run admission context is required")
	}
	if err := ctx.Err(); err != nil {
		return AdmissionProbeResult{}, err
	}

	input = cloneAdmissionInput(input)
	if err := validateAdmissionInput(input); err != nil {
		return AdmissionProbeResult{}, errors.New("certification run admission input is invalid")
	}
	key, err := run.snapshot.key(input)
	if err != nil {
		return AdmissionProbeResult{}, errors.New("certification run admission input is invalid")
	}
	if _, declared := run.scopes[key]; !declared {
		return AdmissionProbeResult{}, errors.New("certification run admission input is not a declared scope")
	}
	return run.snapshot.Probe(ctx, input)
}

// Admit probes every required scope concurrently through the one run-scoped
// snapshot. The worker pool is bounded, results retain input order, and the
// snapshot still deduplicates equal full inputs. Provider-side failures are
// deliberately reported as blocked admission by CheckAdmissionWithProbe.
func (run *CertificationRunAdmission) Admit(ctx context.Context) (CertificationRunAdmissionReport, error) {
	if run == nil || run.snapshot == nil || len(run.inputs) == 0 || run.maxConcurrent < 1 {
		return CertificationRunAdmissionReport{}, errors.New("certification run admission is required")
	}
	if ctx == nil {
		return CertificationRunAdmissionReport{}, errors.New("certification run admission context is required")
	}
	if err := ctx.Err(); err != nil {
		return CertificationRunAdmissionReport{}, err
	}

	report := CertificationRunAdmissionReport{
		Ready:  true,
		Scopes: make([]CertificationRunAdmissionScopeReport, len(run.inputs)),
	}
	for index, input := range run.inputs {
		report.Scopes[index] = CertificationRunAdmissionScopeReport{
			InputIndex:          index,
			Provider:            input.Provider,
			AccountOrProjectRef: input.AccountOrProjectRef,
			Region:              input.Region,
		}
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	workerCount := run.maxConcurrent
	if workerCount > len(run.inputs) {
		workerCount = len(run.inputs)
	}

	var workers sync.WaitGroup
	var errorMu sync.Mutex
	var firstErr error
	setError := func(err error) {
		if err == nil {
			return
		}
		errorMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errorMu.Unlock()
	}

	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-workCtx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					admission, err := CheckAdmissionWithProbe(workCtx, run.inputs[index], run)
					if err != nil {
						setError(err)
						return
					}
					report.Scopes[index].Admission = admission
				}
			}
		}()
	}

dispatch:
	for index := range run.inputs {
		select {
		case jobs <- index:
		case <-workCtx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()

	if err := ctx.Err(); err != nil {
		return report, err
	}
	errorMu.Lock()
	err := firstErr
	errorMu.Unlock()
	if err != nil {
		return report, err
	}
	for index := range report.Scopes {
		if !report.Scopes[index].Admission.Ready {
			report.Ready = false
		}
	}
	return report, nil
}
