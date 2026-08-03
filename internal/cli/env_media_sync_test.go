package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/mediasync"
)

func TestEnvMediaSyncUploadsFixtureListingDiffEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
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
	var sawSource string
	o.mediaSync = func(_ context.Context, opts mediasync.Options) (mediasync.Result, error) {
		sawSource = opts.Source
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
	if sawSource != absSource {
		t.Fatalf("source = %q, want %q", sawSource, absSource)
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

func TestEnvMediaSyncRequiresSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
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
