package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/mediasync"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

func registerMediaTestShim(t *testing.T, o *options, caller *scriptedCaller) {
	t.Helper()
	client := providerhost.NewTestClient(caller, testDescribe())
	module, err := providerhost.NewShimModule("gcp", "gke-autopilot", client)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.modules.RegisterModule(module); err != nil {
		t.Fatal(err)
	}
}

func TestEnvMediaSyncUploadsFixtureListingDiffEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join("..", "..", "testdata", "fixtures", "migrate", "media")
	absSource, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	var sawSource, sawPrefix string
	o.mediaSync = func(_ context.Context, opts mediasync.Options) (mediasync.Result, error) {
		sawSource, sawPrefix = opts.Source, opts.Prefix
		return mediasync.Result{
			Uploaded: 1,
			Keys:     []string{"catalog/product/fixture.txt"},
			Diff:     mediasync.ListingDiff{},
		}, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "media-sync", "staging", "--source", absSource})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if sawSource != absSource || sawPrefix != "media/" {
		t.Fatalf("source = %q, prefix = %q", sawSource, sawPrefix)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if result["uploaded"] != float64(1) {
		t.Fatalf("uploaded = %#v", result["uploaded"])
	}
	diff, _ := result["listingDiff"].(map[string]any)
	if diff["empty"] != true {
		t.Fatalf("listingDiff = %#v", result["listingDiff"])
	}
}

func TestEnvMediaSyncGCPRoutesThroughPlugin(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"mediaBucket": "shop-media"}}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	var sawOp string
	registerMediaTestShim(t, o, &scriptedCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: sdk.StoredPlan{StackName: "shop-preview", Provider: "gcp", Runtime: "gke-autopilot", Tier: sdk.ExtensionTierBuiltIn, ImageDigest: "digest"}}
		case "Plugin.MediaImport":
			sawOp = method
			*(reply.(*sdk.MediaTransferResult)) = sdk.MediaTransferResult{FileCount: 2, ByteCount: 7, LocalDir: t.TempDir()}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}})
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	source := t.TempDir()
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "media-sync", "staging", "--source", source})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if sawOp != "Plugin.MediaImport" {
		t.Fatalf("op = %q, want Plugin.MediaImport", sawOp)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v: %s", err, out.String())
	}
	if result["files"] != float64(2) || result["bucket"] != "shop-media" {
		t.Fatalf("result = %#v", result)
	}
}

func TestEnvMediaExportGCPDownloads(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"mediaBucket": "shop-media"}}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	var sawOp string
	registerMediaTestShim(t, o, &scriptedCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: sdk.StoredPlan{StackName: "shop-preview", Provider: "gcp", Runtime: "gke-autopilot", Tier: sdk.ExtensionTierBuiltIn, ImageDigest: "digest"}}
		case "Plugin.MediaExport":
			sawOp = method
			*(reply.(*sdk.MediaTransferResult)) = sdk.MediaTransferResult{FileCount: 1, ByteCount: 3, LocalDir: t.TempDir()}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}})
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	dest := t.TempDir()
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "media-export", "staging", "--dest", dest})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if sawOp != "Plugin.MediaExport" {
		t.Fatalf("op = %q, want Plugin.MediaExport", sawOp)
	}
}

func TestEnvMediaExportRefusesUnsupportedTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"mediaBucket": "shop-media"}}
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "env", "media-export", "staging", "--dest", t.TempDir()})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unexpected planned type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnvMediaSyncRequiresSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommandWithOptions(testOptions(&out, &fakeTerminal{interactive: false}))
	cmd.SetArgs([]string{"--config", path, "env", "media-sync", "staging"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "source") && !strings.Contains(err.Error(), "required") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvMediaSyncHelpDocumentsMergeAndKeyMapping(t *testing.T) {
	var out bytes.Buffer
	cmd := newCommandWithOptions(testOptions(&out, &fakeTerminal{interactive: false}))
	cmd.SetArgs([]string{"env", "media-sync", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"merge", "pub/media/", "media root", "--source"} {
		if !strings.Contains(strings.ToLower(help), strings.ToLower(want)) && !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}
