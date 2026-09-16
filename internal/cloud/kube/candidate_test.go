package kube

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/platform"
)

func TestMigrationJobBindsRabbitMQCredentials(t *testing.T) {
	job := migrationJob("magelift-migrate-test", CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		DatabaseSecretName:      "shop-db-credentials",
		EncryptionKeySecretName: "shop-encryption-key",
		QueuePasswordSecretName: "shop-queue-password",
		CacheEndpoint:           "cache.internal",
		SearchEndpoint:          "search.internal",
		QueueMode:               "rabbitmq",
		QueueHost:               "rabbitmq",
		QueueUsername:           "magento",
	})

	sawPassword := false
	sawUsername := false
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == platform.EnvMagentoQueueUsername {
			if env.Value != "magento" || env.ValueFrom != nil {
				t.Fatalf("queue username binding = %#v", env)
			}
			sawUsername = true
			continue
		}
		if env.Name != platform.EnvMagentoQueuePassword {
			continue
		}
		if env.Value != "" || env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("queue password env contains a literal or no SecretKeyRef: %#v", env)
		}
		ref := env.ValueFrom.SecretKeyRef
		if ref.Name != "shop-queue-password" || ref.Key != "password" {
			t.Fatalf("queue password SecretKeyRef = %#v", ref)
		}
		sawPassword = true
	}
	if !sawUsername {
		t.Fatalf("migration Job is missing %s", platform.EnvMagentoQueueUsername)
	}
	if !sawPassword {
		t.Fatalf("migration Job is missing %s", platform.EnvMagentoQueuePassword)
	}
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == platform.EnvMagentoSearchHost && env.Value == "search.internal" {
			return
		}
	}
	t.Fatalf("migration Job is missing %s", platform.EnvMagentoSearchHost)
}

func TestProbeJobShape(t *testing.T) {
	command := []string{"/bin/sh", "-ec", "bin/magento setup:db:status"}
	job := probeJob("magelift-probe-test", CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "ghcr.io/magelift/magento@sha256:" + strings.Repeat("e", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		DatabaseSecretName:      "shop-db-credentials",
		EncryptionKeySecretName: "shop-encryption-key",
		CacheEndpoint:           "cache.internal",
	}, command)
	if job.Labels["magelift.io/workload"] != "probe" {
		t.Fatalf("probe job labels = %#v", job.Labels)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Fatalf("probe backoff limit = %#v, want 0 (fail fast, no retries)", job.Spec.BackoffLimit)
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds <= 0 {
		t.Fatalf("probe job has no active deadline: %#v", job.Spec.ActiveDeadlineSeconds)
	}
	containers := job.Spec.Template.Spec.Containers
	if len(containers) != 1 || containers[0].Name != "probe" {
		t.Fatalf("probe containers = %#v", containers)
	}
	got := strings.Join(containers[0].Command, " ")
	if !strings.Contains(got, "setup:db:status") {
		t.Fatalf("probe command = %q", got)
	}
}

func probeCandidateRequest() CandidateRequest {
	return CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "ghcr.io/magelift/magento@sha256:" + strings.Repeat("f", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		DatabaseSecretName:      "shop-db-credentials",
		EncryptionKeySecretName: "shop-encryption-key",
		CacheEndpoint:           "cache.internal",
	}
}

func TestRunProbeLifecycle(t *testing.T) {
	jobs := &fakeJobs{}
	store, err := NewCandidateFromClient(jobs)
	if err != nil {
		t.Fatal(err)
	}
	command := []string{"/bin/sh", "-ec", "bin/magento setup:db:status"}
	if err := store.RunProbe(context.Background(), probeCandidateRequest(), command); err != nil {
		t.Fatalf("RunProbe: %v", err)
	}
	if len(jobs.created) != 1 || len(jobs.waited) != 1 || len(jobs.deleted) != 1 {
		t.Fatalf("created=%d waited=%d deleted=%d", len(jobs.created), len(jobs.waited), len(jobs.deleted))
	}
	if !strings.HasPrefix(jobs.created[0].Name, "magelift-probe-") {
		t.Fatalf("probe job name = %q", jobs.created[0].Name)
	}
}

type failingJobs struct {
	fakeJobs
	err error
}

func (f *failingJobs) WaitJob(_ context.Context, _, _ string, _ time.Duration) error {
	return f.err
}

func TestRunProbeFailurePropagates(t *testing.T) {
	jobs := &failingJobs{err: errors.New("setup:db:status exit 1")}
	store, err := NewCandidateFromClient(jobs)
	if err != nil {
		t.Fatal(err)
	}
	err = store.RunProbe(context.Background(), probeCandidateRequest(), []string{"/bin/sh", "-ec", "bin/magento setup:db:status"})
	if err == nil || !strings.Contains(err.Error(), "setup:db:status exit 1") {
		t.Fatalf("RunProbe error = %v, want probe output attached", err)
	}
	if len(jobs.deleted) != 1 {
		t.Fatalf("failed probe job was not cleaned up: deleted=%v", jobs.deleted)
	}
}

type timeoutCaptureJobs struct {
	fakeJobs
	waitedTimeout time.Duration
}

func (f *timeoutCaptureJobs) WaitJob(_ context.Context, _, _ string, timeout time.Duration) error {
	f.waitedTimeout = timeout
	return nil
}

func TestRunProbeBoundsWait(t *testing.T) {
	jobs := &timeoutCaptureJobs{}
	store, err := NewCandidateFromClient(jobs)
	if err != nil {
		t.Fatal(err)
	}
	store.probeTimeout = 20 * time.Millisecond
	if err := store.RunProbe(context.Background(), probeCandidateRequest(), []string{"/bin/sh", "-ec", "true"}); err != nil {
		t.Fatalf("RunProbe: %v", err)
	}
	if jobs.waitedTimeout != 20*time.Millisecond {
		t.Fatalf("probe wait timeout = %s, want the injected bound", jobs.waitedTimeout)
	}
	defaultJobs := &timeoutCaptureJobs{}
	untouched, err := NewCandidateFromClient(defaultJobs)
	if err != nil {
		t.Fatal(err)
	}
	if err := untouched.RunProbe(context.Background(), probeCandidateRequest(), []string{"/bin/sh", "-ec", "true"}); err != nil {
		t.Fatalf("RunProbe: %v", err)
	}
	if defaultJobs.waitedTimeout != 5*time.Minute {
		t.Fatalf("default probe wait timeout = %s, want 5m", defaultJobs.waitedTimeout)
	}
}

func TestValidateCandidateRequestRequiresRabbitMQSecret(t *testing.T) {
	err := validateCandidateRequest(CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		CacheEndpoint:           "cache.internal",
		QueueMode:               "rabbitmq",
		QueueHost:               "rabbitmq",
		EncryptionKeySecretName: "shop-encryption-key",
	})
	if err == nil || !strings.Contains(err.Error(), "queue password Secret") {
		t.Fatalf("expected queue password Secret validation error, got %v", err)
	}
}
