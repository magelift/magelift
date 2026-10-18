package operations

import (
	"context"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/magelift/magelift/internal/platform"
)

type fakeJobs struct {
	created  []*batchv1.Job
	waited   []string
	deleted  []string
	failWait bool
}

func (f *fakeJobs) CreateJob(_ context.Context, _ string, job *batchv1.Job) (string, error) {
	f.created = append(f.created, job)
	return job.Name, nil
}
func (f *fakeJobs) WaitJob(_ context.Context, _, name string, _ time.Duration) error {
	f.waited = append(f.waited, name)
	if f.failWait {
		return context.DeadlineExceeded
	}
	return nil
}
func (f *fakeJobs) DeleteJob(_ context.Context, _, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

func TestRegisterCandidateUsesPlatformMigrationContract(t *testing.T) {
	t.Parallel()
	jobs := &fakeJobs{}
	store, err := NewDeploymentFromClient(jobs)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.RegisterCandidate(context.Background(), CandidateRequest{
		Project: "example-gcp-project", Region: "europe-west1", Cluster: "shop-gke",
		ImageDigest:    "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		DatabaseWriter: "10.0.0.1", DatabaseName: "magento", CacheEndpoint: "10.0.0.2",
		DatabaseSecretName:      "shop-preview-app-db-credentials",
		EncryptionKeySecretName: "shop-preview-app-encryption-key",
		SearchEndpoint:          "search.internal",
		ApplicationMode:         "integrated", WebRuntime: "nginx-fpm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.JobName == "" || len(jobs.created) != 1 {
		t.Fatalf("candidate=%#v created=%d", candidate, len(jobs.created))
	}
	job := jobs.created[0]
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected one container, got %#v", job.Spec.Template.Spec.Containers)
	}
	container := job.Spec.Template.Spec.Containers[0]
	cmd := strings.Join(container.Command, " ")
	for _, want := range []string{"app:config:import", "setup:upgrade --keep-generated", "cache:clean", "cache:flush"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("command missing %q: %v", want, container.Command)
		}
	}
	if strings.Contains(cmd, "setup:static-content:deploy") {
		t.Fatalf("migration must not regenerate static content at deploy time: %v", container.Command)
	}
	if upgrade, importAt := strings.Index(cmd, "setup:upgrade"), strings.Index(cmd, "app:config:import"); upgrade < 0 || importAt < 0 || upgrade > importAt {
		t.Fatalf("command = %q, want setup:upgrade before app:config:import", cmd)
	}
	foundDBHost := false
	for _, env := range container.Env {
		if env.Name == platform.EnvMagentoDBHost && env.Value == "10.0.0.1" {
			foundDBHost = true
		}
	}
	if !foundDBHost {
		t.Fatalf("missing %s env: %#v", platform.EnvMagentoDBHost, container.Env)
	}
	foundSearchHost := false
	for _, env := range container.Env {
		if env.Name == platform.EnvMagentoSearchHost && env.Value == "search.internal" {
			foundSearchHost = true
		}
	}
	if !foundSearchHost {
		t.Fatalf("missing %s env: %#v", platform.EnvMagentoSearchHost, container.Env)
	}
	for _, want := range []struct {
		name string
		key  string
	}{
		{platform.EnvMagentoDBUser, "username"},
		{platform.EnvMagentoDBPass, "password"},
	} {
		var found bool
		for _, env := range container.Env {
			if env.Name != want.name {
				continue
			}
			found = true
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil || env.ValueFrom.SecretKeyRef.Name != "shop-preview-app-db-credentials" || env.ValueFrom.SecretKeyRef.Key != want.key {
				t.Fatalf("%s secret binding = %#v", env.Name, env.ValueFrom)
			}
		}
		if !found {
			t.Fatalf("missing %s secret binding", want.name)
		}
	}
}

func TestRunMigrationsAndCleanup(t *testing.T) {
	t.Parallel()
	jobs := &fakeJobs{}
	store, err := NewDeploymentFromClient(jobs)
	if err != nil {
		t.Fatal(err)
	}
	candidate := Candidate{Namespace: "default", JobName: "magelift-migrate-test", Cluster: "c", Project: "p", Region: "r"}
	if err := store.RunMigrations(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if len(jobs.waited) != 1 || len(jobs.deleted) != 1 {
		t.Fatalf("waited=%v deleted=%v", jobs.waited, jobs.deleted)
	}
}
