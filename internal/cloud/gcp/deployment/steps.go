// Package deployment adapts GKE Autopilot to the provider-neutral Magento
// deployflow.Steps port. Magento sequencing stays in internal/deploy;
// Magento CLI contracts stay in internal/platform.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	gcpoperations "github.com/acourtiol/magelift/internal/cloud/gcp/operations"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type Backend interface {
	automation.Backend
	Outputs(context.Context) (map[string]any, error)
}

type CandidateRunner interface {
	RegisterCandidate(context.Context, gcpoperations.CandidateRequest) (gcpoperations.Candidate, error)
	RunMigrations(context.Context, gcpoperations.Candidate) error
	Cleanup(context.Context, gcpoperations.Candidate) error
}

type RuntimeChecker interface {
	Check(context.Context, string, string) (gcpoperations.ServiceHealth, error)
}

type Steps struct {
	backend       Backend
	spec          gcpstack.Spec
	candidate     CandidateRunner
	runtime       RuntimeChecker
	diagnostics   io.Writer
	waitInterval  time.Duration
	waitTimeout   time.Duration
	registered    gcpoperations.Candidate
	registeredSet bool
	record        func(context.Context, deployflow.Request, deployflow.Result) error
}

func New(
	backend Backend,
	spec gcpstack.Spec,
	candidate CandidateRunner,
	runtime RuntimeChecker,
	diagnostics io.Writer,
	record func(context.Context, deployflow.Request, deployflow.Result) error,
) (*Steps, error) {
	if backend == nil || candidate == nil || runtime == nil || diagnostics == nil {
		return nil, errors.New("GCP deployment backend, candidate runner, runtime checker, and diagnostics are required")
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("validate GCP deployment spec: %w", err)
	}
	return &Steps{
		backend: backend, spec: spec, candidate: candidate, runtime: runtime,
		diagnostics: diagnostics, waitInterval: 5 * time.Second, waitTimeout: 30 * time.Minute,
		record: record,
	}, nil
}

func (s *Steps) Validate(_ context.Context, request deployflow.Request) error {
	if s == nil || s.backend == nil {
		return errors.New("GCP deployment steps are required")
	}
	if err := sdk.ValidateTargetDescriptor(request.Target); err != nil {
		return err
	}
	if request.ImageDigest != s.spec.Artifact.ImageDigest {
		return fmt.Errorf("deployment digest %q does not match the planned artifact", request.ImageDigest)
	}
	return nil
}

func (s *Steps) Preview(ctx context.Context, request deployflow.Request) (automation.ChangeSummary, error) {
	return automation.NewRunner(s.backend, s.diagnostics).Preview(ctx, automation.Request{Target: request.Target})
}

func (s *Steps) RegisterCandidate(ctx context.Context, request deployflow.Request) error {
	outputs, err := s.backend.Outputs(ctx)
	if err != nil {
		return fmt.Errorf("read GKE deployment outputs: %w", err)
	}
	// Greenfield stacks have no cluster yet. Create infrastructure first.
	if _, err := platform.RequireStringOutput(outputs, platform.OutputClusterName); err != nil {
		if _, updateErr := s.UpdateServices(ctx, request); updateErr != nil {
			return fmt.Errorf("create initial infrastructure: %w", updateErr)
		}
		outputs, err = s.backend.Outputs(ctx)
		if err != nil {
			return fmt.Errorf("read GKE deployment outputs after initial create: %w", err)
		}
	}
	cluster, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil {
		return err
	}
	service, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return err
	}
	databaseWriter, err := platform.RequireStringOutput(outputs, platform.OutputDatabaseWriter)
	if err != nil {
		return err
	}
	cacheEndpoint, err := platform.RequireStringOutput(outputs, platform.OutputCacheEndpoint)
	if err != nil {
		return err
	}
	s.registered, err = s.candidate.RegisterCandidate(ctx, gcpoperations.CandidateRequest{
		Project:         s.spec.Identity.GCPProject,
		Region:          s.spec.Identity.Region,
		Cluster:         cluster,
		Namespace:       "default",
		ServiceName:     service,
		ImageDigest:     request.ImageDigest,
		DatabaseWriter:  databaseWriter,
		DatabaseName:    s.spec.Dependencies.DatabaseName,
		CacheEndpoint:   cacheEndpoint,
		ApplicationMode: s.spec.Application.Mode,
		WebRuntime:      s.spec.Application.WebRuntime,
		CPURequest:      s.spec.Catalog.AutopilotCPURequest,
		MemoryRequest:   s.spec.Catalog.AutopilotMemoryRequest,
	})
	s.registeredSet = err == nil
	return err
}

func (s *Steps) RunMigrations(ctx context.Context, _ deployflow.Request) error {
	if !s.registeredSet {
		return errors.New("deployment candidate has not been registered")
	}
	runErr := s.candidate.RunMigrations(ctx, s.registered)
	cleanupErr := s.candidate.Cleanup(ctx, s.registered)
	if runErr != nil {
		if cleanupErr != nil {
			return fmt.Errorf("run Magento migrations: %w; clean up candidate: %v", runErr, cleanupErr)
		}
		s.registeredSet = false
		return runErr
	}
	if cleanupErr == nil {
		s.registeredSet = false
	}
	return cleanupErr
}

func (s *Steps) CleanupCandidate(ctx context.Context, _ deployflow.Request) error {
	if s == nil || s.candidate == nil || !s.registeredSet {
		return nil
	}
	if err := s.candidate.Cleanup(ctx, s.registered); err != nil {
		return err
	}
	s.registeredSet = false
	return nil
}

func (s *Steps) UpdateServices(ctx context.Context, request deployflow.Request) (automation.ChangeSummary, error) {
	return automation.NewRunner(s.backend, s.diagnostics).Update(ctx, automation.Request{Target: request.Target})
}

func (s *Steps) Stabilize(ctx context.Context, _ deployflow.Request) error {
	return s.waitForHealthyService(ctx)
}

func (s *Steps) Health(ctx context.Context, _ deployflow.Request) error {
	return s.waitForHealthyService(ctx)
}

func (s *Steps) Record(ctx context.Context, request deployflow.Request, result deployflow.Result) error {
	if s.record == nil {
		return nil
	}
	return s.record(ctx, request, result)
}

func (s *Steps) waitForHealthyService(ctx context.Context) error {
	outputs, err := s.backend.Outputs(ctx)
	if err != nil {
		return fmt.Errorf("read GKE runtime outputs: %w", err)
	}
	cluster, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil {
		return err
	}
	service, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return err
	}
	interval, timeout := s.waitInterval, s.waitTimeout
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		health, checkErr := s.runtime.Check(waitContext, cluster, service)
		if checkErr == nil && health.DesiredReplicas > 0 && health.ReadyReplicas >= health.DesiredReplicas && health.Available {
			return nil
		}
		select {
		case <-waitContext.Done():
			if checkErr != nil {
				return fmt.Errorf("wait for GKE deployment stabilization: %w", checkErr)
			}
			return fmt.Errorf(
				"wait for GKE deployment stabilization: desired=%d ready=%d available=%v",
				health.DesiredReplicas, health.ReadyReplicas, health.Available,
			)
		case <-ticker.C:
		}
	}
}
