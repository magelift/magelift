package bootstrap

import (
	"context"
	"testing"
)

type fakeBuckets struct {
	exists  bool
	created bool
	version bool
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

func TestBuildPlanAndEnsure(t *testing.T) {
	t.Parallel()
	plan, err := BuildPlan(Spec{
		Project: "shop", Environment: "preview",
		GCPProject: "digital-lab-341608", Region: "europe-west1",
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
	if !buckets.created || !buckets.version {
		t.Fatalf("created=%v version=%v", buckets.created, buckets.version)
	}
}
