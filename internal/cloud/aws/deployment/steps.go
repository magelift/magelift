// Package deployment adapts the AWS ECS candidate-task workflow to the
// provider-neutral deployment orchestrator.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type Backend interface {
	automation.Backend
	Outputs(context.Context) (map[string]any, error)
}

type CandidateRunner interface {
	RegisterCandidate(context.Context, awsoperations.CandidateRequest) (awsoperations.Candidate, error)
	RunMigrations(context.Context, awsoperations.Candidate) error
	Cleanup(context.Context, awsoperations.Candidate) error
}

type RuntimeChecker interface {
	Check(context.Context, string, string) (awsoperations.ServiceHealth, error)
}

type Steps struct {
	backend       Backend
	spec          awsstack.Spec
	candidate     CandidateRunner
	runtime       RuntimeChecker
	diagnostics   io.Writer
	waitInterval  time.Duration
	waitTimeout   time.Duration
	registered    awsoperations.Candidate
	registeredSet bool
	record        func(context.Context, deployflow.Request, deployflow.Result) error
}

func New(backend Backend, spec awsstack.Spec, candidate CandidateRunner, runtime RuntimeChecker, diagnostics io.Writer, record func(context.Context, deployflow.Request, deployflow.Result) error) (*Steps, error) {
	if backend == nil || candidate == nil || runtime == nil || diagnostics == nil {
		return nil, errors.New("AWS deployment backend, candidate runner, runtime checker, and diagnostics are required")
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("validate AWS deployment spec: %w", err)
	}
	return &Steps{backend: backend, spec: spec, candidate: candidate, runtime: runtime, diagnostics: diagnostics, waitInterval: 5 * time.Second, waitTimeout: 30 * time.Minute, record: record}, nil
}

func (s *Steps) Validate(_ context.Context, request deployflow.Request) error {
	if s == nil || s.backend == nil {
		return errors.New("AWS deployment steps are required")
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
		return fmt.Errorf("read ECS deployment outputs: %w", err)
	}
	cluster, err := requiredString(outputs, "clusterName")
	if err != nil {
		return err
	}
	definition, err := requiredString(outputs, "deployTaskDefinitionArn")
	if err != nil {
		return err
	}
	securityGroup, err := requiredString(outputs, "securityGroupId")
	if err != nil {
		return err
	}
	subnets, err := requiredStrings(outputs, "privateSubnetIds")
	if err != nil {
		return err
	}
	s.registered, err = s.candidate.RegisterCandidate(ctx, awsoperations.CandidateRequest{Cluster: cluster, TaskDefinitionARN: definition, ImageDigest: request.ImageDigest, PrivateSubnetIDs: subnets, SecurityGroupID: securityGroup, StartedBy: "magelift"})
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
		return fmt.Errorf("read ECS runtime outputs: %w", err)
	}
	cluster, err := requiredString(outputs, "clusterName")
	if err != nil {
		return err
	}
	service, err := requiredString(outputs, "serviceName")
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
		if checkErr == nil && health.DesiredCount > 0 && health.RunningCount >= health.DesiredCount && health.PrimaryRollout == "COMPLETED" {
			return nil
		}
		select {
		case <-waitContext.Done():
			if checkErr != nil {
				return fmt.Errorf("wait for ECS service stabilization: %w", checkErr)
			}
			return fmt.Errorf("wait for ECS service stabilization: desired=%d running=%d rollout=%s", health.DesiredCount, health.RunningCount, health.PrimaryRollout)
		case <-ticker.C:
		}
	}
}

func requiredString(outputs map[string]any, key string) (string, error) {
	value, ok := outputs[key]
	if !ok {
		return "", fmt.Errorf("Pulumi output %q is required", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("Pulumi output %q must be a non-empty string", key)
	}
	return text, nil
}

func requiredStrings(outputs map[string]any, key string) ([]string, error) {
	value, ok := outputs[key]
	if !ok {
		return nil, fmt.Errorf("Pulumi output %q is required", key)
	}
	var values []string
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("Pulumi output %q contains a non-string value", key)
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("Pulumi output %q must be a string list", key)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("Pulumi output %q must not be empty", key)
	}
	return values, nil
}
