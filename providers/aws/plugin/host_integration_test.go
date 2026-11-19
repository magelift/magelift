package plugin

import (
	"context"
	"net/url"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestInstalledAWSProviderPlansThroughTheHost(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "magelift-provider-aws")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../cmd/magelift-provider-aws").CombinedOutput(); err != nil {
		t.Fatalf("build AWS provider: %v\n%s", err, out)
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ENDPOINT_URL", "http://127.0.0.1:1")
	localBackend := (&url.URL{Scheme: "file", Path: filepath.ToSlash(t.TempDir())}).String()
	t.Setenv("PULUMI_BACKEND_URL", localBackend)
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "synthetic-local-passphrase")
	t.Setenv("PULUMI_SKIP_UPDATE_CHECK", "true")
	client, err := providerhost.Dial(t.Context(), binary, providerhost.DialOptions{ExpectedProvider: "aws", Required: []sdk.Operation{sdk.OpPlan, sdk.OpPreview, sdk.OpCostInputs}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	for _, runtime := range []string{"ecs-fargate", "eks"} {
		t.Run(runtime, func(t *testing.T) {
			req := shopRequest(t, runtime)
			target, err := parseTarget(req.TargetBlock)
			if err != nil {
				t.Fatal(err)
			}
			cfg := projectConfig(req, target)
			shim, err := providerhost.NewLazyProviderShimModule("aws", sdk.RuntimeID(runtime), platform.TierExperimental, platform.RequiredOutputKeys(), func(context.Context) (*providerhost.Client, error) { return client, nil })
			if err != nil {
				t.Fatal(err)
			}
			planned, err := shim.Plan(cfg, req.Envelope.Environment, platform.PlanOptions{Context: t.Context()})
			if err != nil {
				t.Fatal(err)
			}
			ep, ok := providerhost.AsEnvelopePlan(planned)
			if !ok || ep.StoredPlan().ImageDigest != target.ImageDigest || planned.StackName() != req.Envelope.StackName {
				t.Fatalf("unexpected planned target %v", planned)
			}
			if _, err := platform.ModuleCostEstimator(shim).Estimate(t.Context(), planned, cfg, platform.CostOptions{}); err != nil {
				t.Fatal(err)
			}
			if runtime == "ecs-fargate" {
				t.Run("attached database", func(t *testing.T) {
					attached := *target
					attached.Existing.Database = &config.AWSExistingDatabase{Provider: "aws", Kind: "database", ExternalID: "shop-db"}
					shop := cfg
					shop.Target.AWS = &attached
					teardown, err := shim.Plan(shop, req.Envelope.Environment, platform.PlanOptions{Context: t.Context(), AllowExpiredPreview: true})
					if err != nil {
						t.Fatal(err)
					}
					report := teardown.(platform.BrownfieldAttach)
					if !reflect.DeepEqual(report.AdoptedResourceLines(), []string{"ADOPT database shop-db"}) || report.RefuseAdoptedMutation(platform.AdoptIntentReplace) == nil {
						t.Fatal("attached database lost its ownership protection")
					}
				})
			}
			t.Run("local Pulumi outputs", func(t *testing.T) {
				if _, err := exec.LookPath("pulumi"); err != nil {
					t.Skip("Pulumi CLI required for file-backend integration")
				}
				backend, err := providerhost.NewPluginBackendForPlan(planned)
				if err != nil {
					t.Fatal(err)
				}
				empty, err := backend.Outputs(t.Context())
				if err != nil || len(empty) != 0 {
					t.Fatalf("initial outputs: %v %v", empty, err)
				}
				st, err := automation.NewInlineStackWithBackend(t.Context(), planned.StackName(), func(c *pulumi.Context) error {
					c.Export("namespace", pulumi.String("shop-preview"))
					c.Export("access", pulumi.ToSecret(pulumi.String("test-private-value")))
					return nil
				}, localBackend)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := st.Up(t.Context()); err != nil {
					t.Fatal(err)
				}
				values, err := backend.RedactedOutputs(t.Context())
				if err != nil || values["namespace"] != "shop-preview" || !reflect.DeepEqual(values["access"], map[string]any{"secret": true}) {
					t.Fatalf("unexpected redacted outputs: %v %v", values, err)
				}
				raw, err := backend.Outputs(t.Context())
				if err != nil || raw["access"] != "test-private-value" {
					t.Fatal("internal consumer could not resolve the marked output", err)
				}
			})
			changed, err := providerhost.CallForPlan[sdk.StackCall, sdk.LifecycleResult](t.Context(), planned, sdk.OpApply, &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: ep.Envelope(), Plan: ep.StoredPlan()})
			if err != nil || changed.Error == nil || changed.Error.Code != sdk.ErrCodeInvalid {
				t.Fatalf("non-admitted apply: %v %v", changed, err)
			}
		})
	}
}
