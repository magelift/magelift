//go:build floci_gcp

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/magelift/magelift/internal/automation"
	gcpops "github.com/magelift/magelift/providers/gcp/ops"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func TestGCPFlociPreviewSweepUsesSharedBucket(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI_GCP") != "1" {
		t.Skip("GCP emulator is required")
	}
	if os.Getenv("STORAGE_EMULATOR_HOST") == "" {
		t.Fatal("set STORAGE_EMULATOR_HOST to the Floci storage endpoint")
	}
	// Pulumi resolves credentials before it opens an emulated GCS bucket.
	t.Setenv("GOOGLE_CREDENTIALS", "")
	t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "floci-test-only")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "test-only-passphrase")
	t.Setenv("PULUMI_HOME", t.TempDir())
	ctx := context.Background()
	server := &Server{Admission: stubAdmission{}}
	base := testPlanRequest()
	base.AccountOnly = true
	base.Envelope.Environment, base.Envelope.EnvironmentClass, base.Envelope.Preset = "preview", "preview", "preview"
	base.Envelope.StackName, base.Envelope.StateBackendURL = "shop-preview-gcp-gke-autopilot", ""
	planned, opErr := server.Plan(ctx, base)
	if opErr != nil {
		t.Fatal(opErr)
	}
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	bucket := client.Bucket(strings.TrimPrefix(planned.Plan.StateBackendURL, "gs://"))
	if err := bucket.Create(ctx, "floci-local", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		it := bucket.Objects(context.Background(), nil)
		for {
			a, err := it.Next()
			if err == iterator.Done || err != nil {
				break
			}
			_ = bucket.Object(a.Name).Delete(context.Background())
		}
		_ = bucket.Delete(context.Background())
	})
	state := gcpops.State{NewManager: func(_ context.Context, bucket, project, environment string) (*gcpstate.Manager, error) {
		return gcpstate.NewGCSFromClient(client, bucket, project, environment)
	}}
	var locks []gcpstack.Spec
	for i := 1; i <= 2; i++ {
		request := testPlanRequest()
		request.StateEnvironment = "preview"
		request.AllowExpiredPreview = true
		request.Envelope.Environment, request.Envelope.EnvironmentClass, request.Envelope.Preset = fmt.Sprintf("preview-pr-%d", i), "preview", "preview"
		request.Envelope.StackName, request.Envelope.StateBackendURL = fmt.Sprintf("shop-preview-pr-%d-gcp-gke-autopilot", i), ""
		request.Envelope.ExpiresAt = "2020-01-01T00:00:00Z"
		request.TargetBlock = []byte("project: example-gcp-project\nregion: europe-west1\n")
		preview, opErr := server.Plan(ctx, request)
		if opErr != nil {
			t.Fatal(opErr)
		}
		if preview.Plan.StateBackendURL != planned.Plan.StateBackendURL {
			t.Fatalf("preview backend %q != base %q", preview.Plan.StateBackendURL, planned.Plan.StateBackendURL)
		}
		var spec gcpstack.Spec
		if err := json.Unmarshal(preview.Plan.Opaque, &spec); err != nil {
			t.Fatal(err)
		}
		if err := state.Lock(ctx, spec, fmt.Sprintf("owner-%d", i)); err != nil {
			t.Fatalf("preview lock: %v", err)
		}
		locks = append(locks, spec)
		stack, err := automation.NewInlineStackWithBackend(ctx, preview.Plan.StackName, func(*pulumi.Context) error { return nil }, preview.Plan.StateBackendURL)
		if err != nil {
			t.Fatal(err)
		}
		metadata := automation.PreviewMetadata{Project: "shop", Repository: "example/shop", PullRequest: int64(i), Environment: request.Envelope.Environment, StackKey: preview.Plan.StackName, Owner: "test", Generation: 1, ExpiresAt: "2020-01-01T00:00:00Z"}
		payload, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if err := stack.SetTag(ctx, "magelift:preview:identity", string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	// The sweeper lists ownership tags outside the provider process.
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE_FILE", "")
	records, err := automation.ListPreviewRecords(ctx, planned.Plan.StateBackendURL, "shop")
	if err != nil {
		t.Fatalf("discover previews in bootstrapped bucket: %v", err)
	}
	if len(records) != 2 || records[0].StackName == records[1].StackName {
		t.Fatalf("preview records = %#v", records)
	}
	if _, err := defaultNewStack(ctx, "missing-preview-stack", locks[0], planned.Plan.StateBackendURL); err == nil {
		t.Fatal("destroy planning unexpectedly created a missing stack")
	}
	for _, spec := range locks {
		if _, err := state.Unlock(ctx, spec); err != nil {
			t.Fatalf("preview unlock: %v", err)
		}
	}
}
