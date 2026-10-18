package providerhost

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This opt-in check stages signed historical release assets using GitHub authentication. It proves
// current acquisition/verification code, not the current provider build or a cloud deployment.
func TestAcquireSignedReleaseThenOffline(t *testing.T) {
	if os.Getenv("MAGELIFT_SIGNED_PROVIDER_ACCEPTANCE") != "1" {
		t.Skip("historical asset download requires MAGELIFT_SIGNED_PROVIDER_ACCEPTANCE=1")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("historical canary contains Linux amd64 assets only")
	}
	const version = "v0.0.0-canary.e840adff5dd5ddfe5f165b2a0d44584a90d4c14d"
	t.Chdir(t.TempDir())
	cache := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	// Draft GitHub assets require authenticated staging; their signed bytes are unchanged.
	staged := t.TempDir()
	binaryAsset := "magelift-provider-gcp_" + strings.TrimPrefix(version, "v") + "_linux_amd64"
	lockAsset := "magelift.providers.lock.linux_amd64"
	cmd := exec.CommandContext(ctx, "gh", "release", "download", version, "--repo", "magelift/magelift", "--dir", staged, "--pattern", lockAsset, "--pattern", binaryAsset, "--pattern", binaryAsset+".sigstore.json")
	if err := cmd.Run(); err != nil {
		t.Fatalf("authenticated staging of historical signed release: %v", err)
	}
	opts := AcquireOptions{Resolve: ResolveOptions{CacheDir: cache}}
	stagedAssets := map[string]string{}
	for _, name := range []string{lockAsset, binaryAsset, binaryAsset + ".sigstore.json"} {
		stagedAssets[ReleaseDownloadBase+"/"+version+"/"+name] = filepath.Join(staged, name)
	}
	opts.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if path, ok := stagedAssets[req.URL.String()]; ok {
			file, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Body: file, Header: make(http.Header), Request: req}, nil
		}
		return http.DefaultTransport.RoundTrip(req)
	})}
	loaded, err := Acquire(ctx, "gcp", version, opts)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider != "gcp" || !strings.Contains(loaded.Artifact.Cosign.Identity, version) {
		t.Fatalf("unexpected loaded identity: %+v", loaded)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "http://127.0.0.1:9")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	networkCalls := 0
	opts.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		networkCalls++
		return nil, errors.New("offline acceptance forbids HTTP")
	})}
	offline, err := Acquire(ctx, "gcp", version, opts)
	if err != nil {
		t.Fatalf("offline acquisition: %v", err)
	}
	if networkCalls != 0 || offline.Binary != loaded.Binary {
		t.Fatalf("offline result=%+v, network calls=%d", offline, networkCalls)
	}
	t.Logf("verified signed %s provider from authenticated staging then offline; no provider execution or cloud mutation", version)
}
