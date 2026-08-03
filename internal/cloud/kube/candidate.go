package kube

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/platform"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var candidateDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

// JobAPI creates, waits on, and deletes Magento migrate Jobs via the Kubernetes API.
type JobAPI interface {
	CreateJob(ctx context.Context, namespace string, job *batchv1.Job) (string, error)
	WaitJob(ctx context.Context, namespace, name string, timeout time.Duration) error
	DeleteJob(ctx context.Context, namespace, name string) error
}

// CandidateStore runs Magento migrate candidates as short-lived Jobs.
type CandidateStore struct {
	newJobsFromOutputs func(map[string]any) (JobAPI, error)
	jobs               JobAPI // test override; when set, ignores newJobsFromOutputs
	waitTimeout        time.Duration
	pollInterval       time.Duration
}

// CandidateRequest describes a Magento migrate Job to register.
type CandidateRequest struct {
	Project         string
	Region          string
	Cluster         string
	Namespace       string
	ServiceName     string
	ImageDigest     string
	DatabaseWriter  string
	DatabaseName    string
	CacheEndpoint   string
	ApplicationMode string
	WebRuntime      string
	CPURequest      string
	MemoryRequest   string
	// Outputs supplies stack outputs for ClientFactory-backed stores (kubeconfig).
	Outputs map[string]any
}

// Candidate is a registered migrate Job handle.
type Candidate struct {
	Project   string
	Region    string
	Cluster   string
	Namespace string
	JobName   string
	Outputs   map[string]any
}

// NewCandidateFromClient builds a store that uses an injected JobAPI (tests / fakes).
func NewCandidateFromClient(jobs JobAPI) (*CandidateStore, error) {
	if jobs == nil {
		return nil, errors.New("kubernetes job client is required")
	}
	return &CandidateStore{jobs: jobs, waitTimeout: 30 * time.Minute, pollInterval: 5 * time.Second}, nil
}

// NewCandidateFromFactory builds Jobs via a clientset from stack outputs.
func NewCandidateFromFactory(factory ClientFactory) *CandidateStore {
	if factory == nil {
		factory = ClientFromOutputs
	}
	return &CandidateStore{
		newJobsFromOutputs: func(outputs map[string]any) (JobAPI, error) {
			client, err := factory(outputs)
			if err != nil {
				return nil, err
			}
			if client == nil {
				return nil, fmt.Errorf("kubernetes client factory returned nil")
			}
			return k8sJobs{client: client, pollInterval: 5 * time.Second}, nil
		},
		waitTimeout:  30 * time.Minute,
		pollInterval: 5 * time.Second,
	}
}

func (s *CandidateStore) RegisterCandidate(ctx context.Context, request CandidateRequest) (Candidate, error) {
	if s == nil {
		return Candidate{}, errors.New("kubernetes candidate store is required")
	}
	if err := validateCandidateRequest(request); err != nil {
		return Candidate{}, err
	}
	jobs, err := s.jobsFor(request)
	if err != nil {
		return Candidate{}, err
	}
	namespace := request.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	name := "magelift-migrate-" + time.Now().UTC().Format("20060102t150405")
	if _, err := jobs.CreateJob(ctx, namespace, migrationJob(name, request)); err != nil {
		return Candidate{}, err
	}
	return Candidate{
		Project: request.Project, Region: request.Region, Cluster: request.Cluster,
		Namespace: namespace, JobName: name, Outputs: request.Outputs,
	}, nil
}

func (s *CandidateStore) RunMigrations(ctx context.Context, candidate Candidate) error {
	if s == nil {
		return errors.New("kubernetes candidate store is required")
	}
	if strings.TrimSpace(candidate.Namespace) == "" || strings.TrimSpace(candidate.JobName) == "" {
		return errors.New("candidate namespace and job name are required")
	}
	jobs, err := s.jobsFor(CandidateRequest{
		Project: candidate.Project, Region: candidate.Region, Cluster: candidate.Cluster,
		Outputs: candidate.Outputs,
	})
	if err != nil {
		return err
	}
	timeout := s.waitTimeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	return jobs.WaitJob(ctx, candidate.Namespace, candidate.JobName, timeout)
}

func (s *CandidateStore) Cleanup(ctx context.Context, candidate Candidate) error {
	if s == nil || strings.TrimSpace(candidate.JobName) == "" {
		return nil
	}
	jobs, err := s.jobsFor(CandidateRequest{
		Project: candidate.Project, Region: candidate.Region, Cluster: candidate.Cluster,
		Outputs: candidate.Outputs,
	})
	if err != nil {
		return err
	}
	namespace := candidate.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	return jobs.DeleteJob(ctx, namespace, candidate.JobName)
}

func validateCandidateRequest(request CandidateRequest) error {
	var problems []error
	if strings.TrimSpace(request.Cluster) == "" {
		problems = append(problems, errors.New("cluster is required"))
	}
	if !candidateDigest.MatchString(request.ImageDigest) {
		problems = append(problems, errors.New("image digest must be repository@sha256:..."))
	}
	if strings.TrimSpace(request.DatabaseWriter) == "" || strings.TrimSpace(request.DatabaseName) == "" {
		problems = append(problems, errors.New("database writer and name are required"))
	}
	if strings.TrimSpace(request.CacheEndpoint) == "" {
		problems = append(problems, errors.New("cache endpoint is required"))
	}
	return errors.Join(problems...)
}

func (s *CandidateStore) jobsFor(request CandidateRequest) (JobAPI, error) {
	if s.jobs != nil {
		return s.jobs, nil
	}
	if s.newJobsFromOutputs == nil {
		return nil, fmt.Errorf("kubernetes job client factory is required")
	}
	return s.newJobsFromOutputs(request.Outputs)
}

// k8sJobs implements JobAPI via client-go (no kubectl dependency).
type k8sJobs struct {
	client       kubernetes.Interface
	pollInterval time.Duration
}

func (k k8sJobs) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) (string, error) {
	if k.client == nil || job == nil {
		return "", fmt.Errorf("kubernetes job client and job spec are required")
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	created, err := k.client.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create migrate Job: %w", err)
	}
	return created.Name, nil
}

func (k k8sJobs) WaitJob(ctx context.Context, namespace, name string, timeout time.Duration) error {
	if k.client == nil {
		return fmt.Errorf("kubernetes job client is required")
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	interval := k.pollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		job, err := k.client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("wait for migrate Job: %w", err)
		}
		for _, condition := range job.Status.Conditions {
			switch condition.Type {
			case batchv1.JobComplete:
				if condition.Status == corev1.ConditionTrue {
					return nil
				}
			case batchv1.JobFailed:
				if condition.Status == corev1.ConditionTrue {
					msg := condition.Message
					if msg == "" {
						msg = condition.Reason
					}
					return fmt.Errorf("wait for migrate Job: job failed: %s", msg)
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait for migrate Job: timed out after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for migrate Job: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (k k8sJobs) DeleteJob(ctx context.Context, namespace, name string) error {
	if k.client == nil {
		return nil
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	propagation := metav1.DeletePropagationBackground
	err := k.client.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete migrate Job: %w", err)
	}
	return nil
}

func migrationJob(name string, request CandidateRequest) *batchv1.Job {
	cpu := request.CPURequest
	if cpu == "" {
		cpu = "500m"
	}
	memory := request.MemoryRequest
	if memory == "" {
		memory = "1Gi"
	}
	bindings := platform.CoreEnvBindings(platform.CapabilityEndpoints{
		ApplicationMode: request.ApplicationMode,
		WebRuntime:      request.WebRuntime,
		DatabaseWriter:  request.DatabaseWriter,
		DatabaseName:    request.DatabaseName,
		CacheEndpoint:   request.CacheEndpoint,
	})
	env := make([]corev1.EnvVar, 0, len(bindings))
	for _, binding := range bindings {
		env = append(env, corev1.EnvVar{Name: binding.Name, Value: binding.Value})
	}
	backoff := int32(0)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"magelift.io/workload": "migrate",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "deploy",
						Image:   request.ImageDigest,
						Command: platform.MagentoMigrationShell(),
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpu),
								corev1.ResourceMemory: resource.MustParse(memory),
							},
						},
						Env: env,
					}},
				},
			},
		},
	}
}
