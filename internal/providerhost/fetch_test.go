package providerhost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func fetchTestLock(version string) string {
	return `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"` + version + `","protocol":"magelift-v1","digest":"sha256:` + strings.Repeat("c", 64) + `","url":"https://example.invalid/x","cosign":{"identity":"` + FirstPartyIdentityPrefix + version + `","issuer":"` + FirstPartyIssuer + `","bundle":"https://example.invalid/x.sigstore.json"}}}}`
}

func TestFetchLockDownloadsAndParses(t *testing.T) {
	version := "v0.1.0-alpha.1-rc.2"
	platform := runtime.GOOS + "_" + runtime.GOARCH
	var gotPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(fetchTestLock(version)))
	}))
	t.Cleanup(server.Close)

	lock, err := FetchLock(context.Background(), version, server.URL, server.Client())
	if err != nil {
		t.Fatalf("FetchLock: %v", err)
	}
	if want := "/" + version + "/magelift.providers.lock." + platform; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
	if _, err := lock.Artifact("gcp"); err != nil {
		t.Fatalf("Artifact: %v", err)
	}
}

func TestFetchLockRefusesForeignPublisher(t *testing.T) {
	version := "v1.2.3"
	lock := strings.Replace(fetchTestLock(version), FirstPartyIdentityPrefix+version, "https://evil.example/wolf", 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(lock))
	}))
	t.Cleanup(server.Close)

	if _, err := FetchLock(context.Background(), version, server.URL, server.Client()); err == nil {
		t.Fatal("foreign publisher was accepted")
	}
}

func TestFetchLockRejectsNonTags(t *testing.T) {
	for _, version := range []string{"", "dev", "main", "latest", "1.2.3", "v1.2", "v1.2.3\n(base64)"} {
		if ValidReleaseTag(version) {
			t.Fatalf("version %q accepted", version)
		}
		if _, err := FetchLock(context.Background(), version, "https://example.invalid", nil); err == nil {
			t.Fatalf("version %q fetched", version)
		}
	}
	for _, version := range []string{"v1.2.3", "v0.1.0-alpha.1-rc.2", "v2.0.0+build.1"} {
		if !ValidReleaseTag(version) {
			t.Fatalf("version %q refused", version)
		}
	}
}

func TestFetchLockRequiresHTTPSBase(t *testing.T) {
	if _, err := FetchLock(context.Background(), "v1.2.3", "http://example.invalid", nil); err == nil {
		t.Fatal("http base was accepted")
	}
}

func TestEffectiveCacheDirPrefersFlagThenEnv(t *testing.T) {
	t.Setenv("MAGELIFT_PROVIDER_CACHE_DIR", "/env/cache")
	if got, _ := EffectiveCacheDir("/flag/cache"); got != "/flag/cache" {
		t.Fatalf("got %q", got)
	}
	if got, _ := EffectiveCacheDir(""); got != "/env/cache" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeReleaseTagRestoresGoReleaserPrefix(t *testing.T) {
	t.Parallel()
	for _, kase := range []struct{ in, want string }{
		{"v0.1.0-alpha.1-rc.2", "v0.1.0-alpha.1-rc.2"},
		{"0.1.0-alpha.1-rc.2", "v0.1.0-alpha.1-rc.2"},
		{"  v1.2.3  ", "v1.2.3"},
		{"1.2.3", "v1.2.3"},
		{"1.2.3+build.5", "v1.2.3+build.5"},
	} {
		tag, err := NormalizeReleaseTag(kase.in)
		if err != nil || tag != kase.want {
			t.Errorf("NormalizeReleaseTag(%q) = %q, %v; want %q", kase.in, tag, err, kase.want)
		}
	}
	for _, bad := range []string{"", "dev", "main", "latest", "v1.2", "1.2", "v1.2.3-../escape", "release/v1.2.3"} {
		if tag, err := NormalizeReleaseTag(bad); err == nil {
			t.Errorf("NormalizeReleaseTag(%q) = %q, want error", bad, tag)
		}
	}
}
