package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
)

type fakeBuckets struct {
	exists     bool
	meta       BucketMetadata
	created    bool
	creates    int
	version    bool
	softDelete bool
}

func (f *fakeBuckets) BucketExists(_ context.Context, _ string) (bool, BucketMetadata, error) {
	if f.exists {
		return true, f.meta, nil
	}
	return false, BucketMetadata{}, nil
}

func (f *fakeBuckets) CreateBucket(_ context.Context, _, _, location string, labels map[string]string) error {
	f.created = true
	f.creates++
	f.exists = true
	f.meta = BucketMetadata{Location: location, Labels: copyStateBucketLabels(labels)}
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

func testPlan(t *testing.T) Plan {
	plan, err := BuildPlan(Spec{
		Project: "shop", Environment: "preview",
		GCPProject: "example-gcp-project", Region: "europe-west1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func ownedBucketMetadata(plan Plan) BucketMetadata {
	return BucketMetadata{
		Location: plan.Region,
		Labels:   copyStateBucketLabels(plan.Labels),
	}
}

func copyStateBucketLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func TestBuildPlanAndEnsure(t *testing.T) {
	t.Parallel()
	plan := testPlan(t)
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
	if _, err := boot.Ensure(context.Background(), plan); err != nil {
		t.Fatalf("repeat after create: %v", err)
	}
	if buckets.creates != 1 {
		t.Fatalf("created %d buckets on repeated bootstrap", buckets.creates)
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

func TestValidateExistingStateBucketFromBucketAttrs(t *testing.T) {
	t.Parallel()
	plan := testPlan(t)
	attrs := &storage.BucketAttrs{
		Location: "europe-west1",
		Labels: map[string]string{
			"magelift-managed-by":  "magelift",
			"magelift-project":     "shop",
			"magelift-environment": "preview",
			"magelift-purpose":     "pulumi-state",
			"unrelated":            "ignored",
		},
	}
	meta := bucketMetadataFromAttrs(attrs)
	if err := validateExistingStateBucket(plan, meta); err != nil {
		t.Fatalf("validateExistingStateBucket: %v", err)
	}
}

func TestValidateExistingStateBucketRejectsForeignAndDrift(t *testing.T) {
	t.Parallel()
	plan := testPlan(t)
	owned := ownedBucketMetadata(plan)

	cases := []struct {
		name string
		meta BucketMetadata
		want string
	}{
		{
			name: "foreign managed-by",
			meta: BucketMetadata{Location: owned.Location, Labels: copyStateBucketLabels(owned.Labels)},
			want: `label "magelift-managed-by"`,
		},
		{
			name: "missing project label",
			meta: BucketMetadata{
				Location: owned.Location,
				Labels: map[string]string{
					"magelift-managed-by":  "magelift",
					"magelift-environment": "preview",
					"magelift-purpose":     "pulumi-state",
				},
			},
			want: `missing ownership label "magelift-project"`,
		},
		{
			name: "wrong region",
			meta: BucketMetadata{Location: "us-central1", Labels: owned.Labels},
			want: `location is "us-central1"`,
		},
	}
	cases[0].meta.Labels["magelift-managed-by"] = "other-tool"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExistingStateBucket(plan, tc.meta)
			if err == nil {
				t.Fatal("expected refusal")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error=%q, want substring %q", msg, tc.want)
			}
			if !strings.Contains(msg, stateBucketOwnershipRefusalHint) {
				t.Fatalf("error=%q, want actionable hint", msg)
			}
		})
	}
}

func TestEnsureRefusesForeignPreExistingBucketWithoutMutation(t *testing.T) {
	t.Parallel()
	plan := testPlan(t)
	buckets := &fakeBuckets{
		exists: true,
		meta: BucketMetadata{
			Location: plan.Region,
			Labels:   map[string]string{"magelift-managed-by": "terraform"},
		},
	}
	boot, err := NewFromClient(buckets)
	if err != nil {
		t.Fatal(err)
	}
	_, err = boot.Ensure(context.Background(), plan)
	if err == nil {
		t.Fatal("expected foreign bucket refusal")
	}
	if buckets.created || buckets.version || buckets.softDelete {
		t.Fatalf("mutated foreign bucket: created=%v version=%v softDelete=%v", buckets.created, buckets.version, buckets.softDelete)
	}
}

func TestEnsureOwnedPreExistingBucketConverges(t *testing.T) {
	t.Parallel()
	plan := testPlan(t)
	buckets := &fakeBuckets{exists: true, meta: ownedBucketMetadata(plan)}
	boot, err := NewFromClient(buckets)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := boot.Ensure(context.Background(), plan); err != nil {
			t.Fatalf("ensure %d: %v", i+1, err)
		}
	}
	if buckets.created {
		t.Fatal("owned bucket must not be recreated")
	}
	if !buckets.version || !buckets.softDelete {
		t.Fatalf("version=%v softDelete=%v", buckets.version, buckets.softDelete)
	}
}

func TestEnsureRefusesEveryOwnershipMismatchBeforeMutation(t *testing.T) {
	t.Parallel()
	plan := testPlan(t)
	for _, field := range []string{"magelift-managed-by", "magelift-project", "magelift-environment", "magelift-purpose", "missing-labels", "region"} {
		t.Run(field, func(t *testing.T) {
			meta := ownedBucketMetadata(plan)
			switch field {
			case "missing-labels":
				meta.Labels = nil
			case "region":
				meta.Location = "us-central1"
			default:
				meta.Labels[field] = "foreign"
			}
			buckets := &fakeBuckets{exists: true, meta: meta}
			boot, err := NewFromClient(buckets)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := boot.Ensure(context.Background(), plan); err == nil {
				t.Fatal("expected ownership refusal")
			}
			if buckets.created || buckets.version || buckets.softDelete {
				t.Fatalf("mutated bucket with %s mismatch", field)
			}
		})
	}
}
