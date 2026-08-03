// Package operations is the GKE day-2 and Magento candidate adapter surface.
// Magento CLI contracts come from internal/platform; sequencing from deployflow.
package operations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
)

var candidateDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

// JobAPI creates, waits on, and deletes Magento migrate Jobs via the Kubernetes API.
type JobAPI interface {
	CreateJob(ctx context.Context, namespace string, job *batchv1.Job) (string, error)
	WaitJob(ctx context.Context, namespace, name string, timeout time.Duration) error
	DeleteJob(ctx context.Context, namespace, name string) error
}

// DeploymentStore runs Magento migrate candidates as short-lived GKE Jobs.
type DeploymentStore struct {
	newJobs     func(project, region, cluster string) JobAPI
	jobs        JobAPI // test override; when set, ignores newJobs
	waitTimeout time.Duration
}

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
}

type Candidate struct {
	Project   string
	Region    string
	Cluster   string
	Namespace string
	JobName   string
}

func NewDeploymentFromClient(jobs JobAPI) (*DeploymentStore, error) {
	if jobs == nil {
		return nil, errors.New("GKE job client is required")
	}
	return &DeploymentStore{jobs: jobs, waitTimeout: 30 * time.Minute}, nil
}

// NewDeployment builds a store that talks to GKE with client-go (ADC), not kubectl.
func NewDeployment(ctx context.Context, project, region string) (*DeploymentStore, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	return &DeploymentStore{
		newJobs: func(p, r, cluster string) JobAPI {
			// Construct on demand with a fresh background context; call sites pass
			// their own ctx into CreateJob/WaitJob/DeleteJob.
			clientset, err := newGKEClientset(context.Background(), p, r, cluster)
			if err != nil {
				return errJobs{err: err}
			}
			return k8sJobs{client: clientset}
		},
		waitTimeout: 30 * time.Minute,
	}, nil
}

// errJobs surfaces client construction failures on the first JobAPI call.
type errJobs struct{ err error }

func (e errJobs) CreateJob(context.Context, string, *batchv1.Job) (string, error) {
	return "", e.err
}
func (e errJobs) WaitJob(context.Context, string, string, time.Duration) error { return e.err }
func (e errJobs) DeleteJob(context.Context, string, string) error              { return e.err }

func (s *DeploymentStore) RegisterCandidate(ctx context.Context, request CandidateRequest) (Candidate, error) {
	if s == nil {
		return Candidate{}, errors.New("GKE deployment client is required")
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
		namespace = "default"
	}
	name := "magelift-migrate-" + time.Now().UTC().Format("20060102t150405")
	if _, err := jobs.CreateJob(ctx, namespace, migrationJob(name, request)); err != nil {
		return Candidate{}, err
	}
	return Candidate{
		Project: request.Project, Region: request.Region, Cluster: request.Cluster,
		Namespace: namespace, JobName: name,
	}, nil
}

func (s *DeploymentStore) RunMigrations(ctx context.Context, candidate Candidate) error {
	if s == nil {
		return errors.New("GKE deployment client is required")
	}
	if strings.TrimSpace(candidate.Namespace) == "" || strings.TrimSpace(candidate.JobName) == "" {
		return errors.New("candidate namespace and job name are required")
	}
	jobs, err := s.jobsFor(CandidateRequest{
		Project: candidate.Project, Region: candidate.Region, Cluster: candidate.Cluster,
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

func (s *DeploymentStore) Cleanup(ctx context.Context, candidate Candidate) error {
	if s == nil || strings.TrimSpace(candidate.JobName) == "" {
		return nil
	}
	jobs, err := s.jobsFor(CandidateRequest{
		Project: candidate.Project, Region: candidate.Region, Cluster: candidate.Cluster,
	})
	if err != nil {
		return err
	}
	namespace := candidate.Namespace
	if namespace == "" {
		namespace = "default"
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

func (s *DeploymentStore) jobsFor(request CandidateRequest) (JobAPI, error) {
	if s.jobs != nil {
		return s.jobs, nil
	}
	if s.newJobs == nil {
		return nil, fmt.Errorf("GKE job client factory is required")
	}
	if strings.TrimSpace(request.Cluster) == "" {
		return nil, fmt.Errorf("cluster is required")
	}
	return s.newJobs(request.Project, request.Region, request.Cluster), nil
}
