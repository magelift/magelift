//go:build floci

package floci_test

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/magelift/magelift/internal/awsendpoint"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

func TestAWSPluginTransfersMediaAndHoldsItsOwnLock(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI") != "1" {
		t.Skip("requires Floci")
	}
	endpoint := os.Getenv("MAGELIFT_FLOCI_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:4566"
	}
	if local, err := awsendpoint.Parse(endpoint); err != nil || local == "" {
		t.Fatalf("invalid Floci endpoint %q: %v", endpoint, err)
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv(awsendpoint.EnvironmentVariable, endpoint)
	ctx := t.Context()
	binary := filepath.Join(t.TempDir(), "magelift-provider-aws")
	if output, err := exec.CommandContext(ctx, "go", "build", "-o", binary, "../cmd/magelift-provider-aws").CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, output)
	}
	client, err := providerhost.Dial(ctx, binary, providerhost.DialOptions{ExpectedProvider: "aws", Required: []sdk.Operation{sdk.OpPlan, sdk.OpMediaImport, sdk.OpMediaExport, sdk.OpStateLock, sdk.OpStateUnlock, sdk.OpSecretSet, sdk.OpSecretRead, sdk.OpSecretRemove}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	const region = "us-east-1"
	buckets, err := awsendpoint.S3Client(ctx, region, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{"ecs-fargate", "eks"} {
		t.Run(runtime, func(t *testing.T) {
			ctx := t.Context()
			env := sdk.Envelope{Project: "plugintest", Environment: "staging", Account: "000000000000", Region: region, EnvironmentClass: "staging", Preset: "standard", StackName: platform.FormatStackName("plugintest", "staging", "aws", sdk.RuntimeID(runtime))}
			plan, err := providerhost.Call[sdk.PlanRequest, sdk.PlanResult](ctx, client, sdk.OpPlan, &sdk.PlanRequest{ProtocolVersion: sdk.ProtocolV1, Runtime: runtime, Envelope: env, AccountOnly: true, TargetBlock: []byte("kmsKeyArn: arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000")})
			if err != nil || plan.Error != nil {
				t.Fatalf("plan: %v %v", plan, err)
			}
			u, err := url.Parse(plan.Plan.StateBackendURL)
			if err != nil || u.Host == "" {
				t.Fatalf("bad backend %s", plan.Plan.StateBackendURL)
			}
			bucket := u.Host
			lockCall := &sdk.StateLockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: env, Plan: plan.Plan, Owner: "first"}
			missing, err := providerhost.Call[sdk.StateLockCall, sdk.StateLockResult](ctx, client, sdk.OpStateLock, lockCall)
			if err != nil || missing.Error == nil || missing.Error.Code != sdk.ErrCodeNotFound || !strings.Contains(missing.Error.Message, "magelift bootstrap --env staging") {
				t.Fatalf("missing state bucket hint: %v %v", missing, err)
			}
			if _, err = buckets.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket}); err != nil {
				t.Fatal(err)
			}
			locked, err := providerhost.Call[sdk.StateLockCall, sdk.StateLockResult](ctx, client, sdk.OpStateLock, lockCall)
			if err != nil || locked.Error != nil || locked.ReleaseToken == "" {
				t.Fatalf("lock: %v %v", locked, err)
			}
			unlock := &sdk.StateUnlockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: env, Plan: plan.Plan}
			release := func(token string) *sdk.OperationError {
				unlock.ReleaseToken = token
				result, err := providerhost.Call[sdk.StateUnlockCall, sdk.StateUnlockResult](ctx, client, sdk.OpStateUnlock, unlock)
				if err != nil {
					t.Fatal(err)
				}
				return result.Error
			}
			if err := release(""); err != nil {
				t.Fatal(err)
			}
			lockCall.Owner = "successor"
			next, err := providerhost.Call[sdk.StateLockCall, sdk.StateLockResult](ctx, client, sdk.OpStateLock, lockCall)
			if err != nil || next.Error != nil {
				t.Fatalf("successor: %v %v", next, err)
			}
			if err := release(locked.ReleaseToken); err == nil {
				t.Fatal("stale token released successor lock")
			}
			if err := release(next.ReleaseToken); err != nil {
				t.Fatal(err)
			}

			name := "magelift/plugintest/staging/" + runtime
			write, err := providerhost.Call[sdk.SecretSetCall, sdk.SecretSetResult](ctx, client, sdk.OpSecretSet, &sdk.SecretSetCall{
				ProtocolVersion: sdk.ProtocolV1, Envelope: env, Plan: plan.Plan, Name: name, Value: []byte("local-fixture")})
			if err != nil || write.Error != nil || !write.Written {
				t.Fatalf("write secret: %v %v", write, err)
			}
			value, err := providerhost.Call[sdk.SecretReadRequest, sdk.SecretReadResult](ctx, client, sdk.OpSecretRead, &sdk.SecretReadRequest{ProtocolVersion: sdk.ProtocolV1, Envelope: env, Name: name})
			if err != nil || value.Error != nil || string(value.Value) != "local-fixture" {
				t.Fatal("AWS secret reference did not return the stored fixture", err)
			}
			removed, err := providerhost.Call[sdk.SecretRemoveCall, sdk.SecretRemoveResult](ctx, client, sdk.OpSecretRemove, &sdk.SecretRemoveCall{ProtocolVersion: sdk.ProtocolV1, Envelope: env, Plan: plan.Plan, Name: name})
			if err != nil || removed.Error != nil || !removed.Removed {
				t.Fatalf("remove secret: %v %v", removed, err)
			}

			source, output := t.TempDir(), t.TempDir()
			const content = "Magento media fixture"
			if err := os.WriteFile(filepath.Join(source, "banner.jpg"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			jsonOutputs, err := json.Marshal(map[string]string{platform.OutputMediaBucket: bucket})
			if err != nil {
				t.Fatal(err)
			}
			call := &sdk.MediaTransferCall{ProtocolVersion: sdk.ProtocolV1, Envelope: env, Plan: plan.Plan, OutputsJSON: jsonOutputs, LocalDir: source}
			result, err := providerhost.Call[sdk.MediaTransferCall, sdk.MediaTransferResult](ctx, client, sdk.OpMediaImport, call)
			if err != nil || result.Error != nil || result.FileCount != 1 || result.ByteCount != int64(len(content)) || result.Listing == nil || strings.Join(result.Listing.Keys, ",") != "media/banner.jpg" {
				t.Fatalf("import: %v %v", result, err)
			}
			call.LocalDir = output
			result, err = providerhost.Call[sdk.MediaTransferCall, sdk.MediaTransferResult](ctx, client, sdk.OpMediaExport, call)
			if err != nil || result.Error != nil || result.FileCount != 1 || result.ByteCount != int64(len(content)) {
				t.Fatalf("export: %v %v", result, err)
			}
			got, err := os.ReadFile(filepath.Join(output, "banner.jpg"))
			if err != nil || string(got) != content {
				t.Fatalf("exported file: %q %v", got, err)
			}
			key := "media/banner.jpg"
			if _, err := buckets.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: &key}); err != nil {
				t.Fatal(err)
			}
			if _, err := buckets.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &bucket}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
