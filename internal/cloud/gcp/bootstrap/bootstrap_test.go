package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeBuckets struct {
	exists     bool
	created    bool
	version    bool
	softDelete bool
}

func (f *fakeBuckets) BucketExists(context.Context, string) (bool, error) { return f.exists, nil }
func (f *fakeBuckets) CreateBucket(_ context.Context, _, _, _ string, _ map[string]string) error {
	f.created = true
	return nil
}
func (f *fakeBuckets) EnsureVersioning(context.Context, string) error {
	f.version = true
	return nil
}
func (f *fakeBuckets) EnsureSoftDelete(_ context.Context, _ string, retention time.Duration) error {
	if retention <= 0 {
		return errors.New("soft-delete retention must be positive")
	}
	f.softDelete = true
	return nil
}

func TestBuildPlanAndEnsure(t *testing.T) {
	t.Parallel()
	plan, err := BuildPlan(Spec{
		Project: "shop", Environment: "preview",
		GCPProject: "example-gcp-project", Region: "europe-west1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.StateBucket == "" || BackendURL(plan) != "gs://"+plan.StateBucket {
		t.Fatalf("plan=%#v", plan)
	}
	buckets := &fakeBuckets{}
	boot, err := NewFromClient(buckets)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boot.Ensure(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if !buckets.created || !buckets.version || !buckets.softDelete {
		t.Fatalf("created=%v version=%v softDelete=%v", buckets.created, buckets.version, buckets.softDelete)
	}
}

func TestBuildPlanCompactsHighAvailabilityStateBucket(t *testing.T) {
	t.Parallel()

	plan, err := BuildPlan(Spec{
		Project:     "mlha1",
		Environment: highAvailabilityEnvironment,
		GCPProject:  "digital-lab-341608",
		Region:      "europe-west1",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "magelift-digital-lab-341608-europe-west1-mlha1-ha-state"
	if plan.StateBucket != want {
		t.Fatalf("state bucket=%q, want %q", plan.StateBucket, want)
	}
}

func TestBuildPlanCompactsLongStateBucketDeterministically(t *testing.T) {
	t.Parallel()

	spec := Spec{
		Project:     "abcdefghijklmnopqrstuvwxyz12345",
		Environment: "preview",
		GCPProject:  "digital-lab-341608",
		Region:      "europe-west1",
	}
	first, err := BuildPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if first.StateBucket != second.StateBucket {
		t.Fatalf("state bucket is not deterministic: first=%q second=%q", first.StateBucket, second.StateBucket)
	}
	if len(first.StateBucket) > maxStateBucketNameLength {
		t.Fatalf("state bucket length=%d, want <=%d: %q", len(first.StateBucket), maxStateBucketNameLength, first.StateBucket)
	}
	if !strings.HasPrefix(first.StateBucket, "magelift-digital-lab-341608-europe-") || len(first.StateBucket) != maxStateBucketNameLength {
		t.Fatalf("state bucket=%q, want readable capped name", first.StateBucket)
	}
	if !strings.HasSuffix(first.StateBucket, "-preview-state") {
		t.Fatalf("state bucket=%q, want environment retained in readable suffix", first.StateBucket)
	}
}
