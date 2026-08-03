package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/automation"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Backend is the Pulumi automation + stack Outputs surface Steps needs.
type Backend interface {
	automation.Backend
	Outputs(context.Context) (map[string]any, error)
}

// CandidateRunner registers and cleans Magento migrate Jobs.
type CandidateRunner interface {
	RegisterCandidate(context.Context, CandidateRequest) (Candidate, error)
	RunMigrations(context.Context, Candidate) error
	Cleanup(context.Context, Candidate) error
}

// ServiceHealth is a Deployment readiness snapshot for Stabilize/Health.
type ServiceHealth struct {
	DesiredReplicas int
	ReadyReplicas   int
	Available       bool
}

// RuntimeChecker reports Magento web Deployment health.
type RuntimeChecker interface {
	Check(context.Context, string, string) (ServiceHealth, error)
}

// DeploySpec holds provider-portable Magento deploy knobs for kube.Steps.
type DeploySpec struct {
	ImageDigest     string
	DatabaseName    string
	ApplicationMode string
	WebRuntime      string
	CPURequest      string
	MemoryRequest   string
	CloudProject    string
	Region          string
}

func (s DeploySpec) Validate() error {
	var problems []error
	if !candidateDigest.MatchString(s.ImageDigest) {
		problems = append(problems, errors.New("image digest must be repository@sha256:..."))
	}
	if strings.TrimSpace(s.DatabaseName) == "" {
		problems = append(problems, errors.New("database name is required"))
	}
	return errors.Join(problems...)
}

// Steps implements deployflow.Steps for Magento on Kubernetes (D-03, KUBE-04).
type Steps struct {
	backend       Backend
	spec          DeploySpec
	candidate     CandidateRunner
	runtime       RuntimeChecker
	diagnostics   io.Writer
	waitInterval  time.Duration
	waitTimeout   time.Duration
	registered    Candidate
	registeredSet bool
	record        func(context.Context, deployflow.Request, deployflow.Result) error
}

// New constructs shared Kubernetes Magento deploy Steps.
func New(
	backend Backend,
	spec DeploySpec,
	candidate CandidateRunner,
	runtime RuntimeChecker,
	diagnostics io.Writer,
	record func(context.Context, deployflow.Request, deployflow.Result) error,
) (*Steps, error) {
	if backend == nil || candidate == nil || runtime == nil || diagnostics == nil {
		return nil, errors.New("kube deployment backend, candidate runner, runtime checker, and diagnostics are required")
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("validate kube deployment spec: %w", err)
	}
	return &Steps{
		backend: backend, spec: spec, candidate: candidate, runtime: runtime,
		diagnostics: diagnostics, waitInterval: 5 * time.Second, waitTimeout: 30 * time.Minute,
		record: record,
	}, nil
}

func (s *Steps) Validate(_ context.Context, request deployflow.Request) error {
	if s == nil || s.backend == nil {
		return errors.New("kube deployment steps are required")
	}
	if err := sdk.ValidateTargetDescriptor(request.Target); err != nil {
		return err
	}
	if request.ImageDigest != s.spec.ImageDigest {
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
		return fmt.Errorf("read kubernetes deployment outputs: %w", err)
	}
	// Greenfield stacks have no cluster yet. Create infrastructure first.
	if _, err := platform.RequireStringOutput(outputs, platform.OutputClusterName); err != nil {
		if _, updateErr := s.UpdateServices(ctx, request); updateErr != nil {
			return fmt.Errorf("create initial infrastructure: %w", updateErr)
		}
		outputs, err = s.backend.Outputs(ctx)
		if err != nil {
			return fmt.Errorf("read kubernetes deployment outputs after initial create: %w", err)
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
	s.registered, err = s.candidate.RegisterCandidate(ctx, CandidateRequest{
		Project:         s.spec.CloudProject,
		Region:          s.spec.Region,
		Cluster:         cluster,
		Namespace:       defaultNamespace,
		ServiceName:     service,
		ImageDigest:     request.ImageDigest,
		DatabaseWriter:  databaseWriter,
		DatabaseName:    s.spec.DatabaseName,
		CacheEndpoint:   cacheEndpoint,
		ApplicationMode: s.spec.ApplicationMode,
		WebRuntime:      s.spec.WebRuntime,
		CPURequest:      s.spec.CPURequest,
		MemoryRequest:   s.spec.MemoryRequest,
		Outputs:         outputs,
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
		return fmt.Errorf("read kubernetes runtime outputs: %w", err)
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
				return fmt.Errorf("wait for kubernetes deployment stabilization: %w", checkErr)
			}
			return fmt.Errorf(
				"wait for kubernetes deployment stabilization: desired=%d ready=%d available=%v",
				health.DesiredReplicas, health.ReadyReplicas, health.Available,
			)
		case <-ticker.C:
		}
	}
}

// DeploymentRuntime checks Magento web Deployment readiness via client-go.
type DeploymentRuntime struct {
	client  kubernetes.Interface
	factory ClientFactory
	backend Backend
}

// NewRuntimeFromClient injects a clientset (tests / pre-built clients).
func NewRuntimeFromClient(client kubernetes.Interface) (*DeploymentRuntime, error) {
	if client == nil {
		return nil, errors.New("kubernetes client is required")
	}
	return &DeploymentRuntime{client: client}, nil
}

// NewRuntimeFromFactory builds a client from stack outputs on each Check.
func NewRuntimeFromFactory(backend Backend, factory ClientFactory) (*DeploymentRuntime, error) {
	if backend == nil {
		return nil, errors.New("kubernetes deployment backend is required")
	}
	if factory == nil {
		factory = ClientFromOutputs
	}
	return &DeploymentRuntime{backend: backend, factory: factory}, nil
}

func (r *DeploymentRuntime) Check(ctx context.Context, _, serviceName string) (ServiceHealth, error) {
	if r == nil {
		return ServiceHealth{}, errors.New("kubernetes runtime checker is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return ServiceHealth{}, errors.New("service name is required")
	}
	client, err := r.clientFor(ctx)
	if err != nil {
		return ServiceHealth{}, err
	}
	return deploymentHealth(ctx, client, defaultNamespace, serviceName)
}

func (r *DeploymentRuntime) clientFor(ctx context.Context) (kubernetes.Interface, error) {
	if r.client != nil {
		return r.client, nil
	}
	if r.backend == nil {
		return nil, errors.New("kubernetes client or backend is required")
	}
	factory := r.factory
	if factory == nil {
		factory = ClientFromOutputs
	}
	outputs, err := r.backend.Outputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read kubernetes runtime outputs: %w", err)
	}
	client, err := factory(outputs)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("kubernetes client factory returned nil")
	}
	return client, nil
}

func deploymentHealth(ctx context.Context, client kubernetes.Interface, namespace, deployment string) (ServiceHealth, error) {
	if client == nil {
		return ServiceHealth{}, errors.New("kubernetes client is required")
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	dep, err := client.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return ServiceHealth{}, fmt.Errorf("get deployment %s: %w", deployment, err)
	}
	desired := 0
	if dep.Spec.Replicas != nil {
		desired = int(*dep.Spec.Replicas)
	}
	available := false
	for _, condition := range dep.Status.Conditions {
		if condition.Type == "Available" && condition.Status == corev1.ConditionTrue {
			available = true
			break
		}
	}
	return ServiceHealth{
		DesiredReplicas: desired,
		ReadyReplicas:   int(dep.Status.ReadyReplicas),
		Available:       available,
	}, nil
}
