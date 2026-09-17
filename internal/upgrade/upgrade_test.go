package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
)

type fakeHTTP struct {
	responses map[string]string
	status    int
}

type fakeBlobVerifier struct {
	err error
}

func (f fakeBlobVerifier) VerifyBlob(context.Context, string, string, cosign.VerifyOptions) error {
	return f.err
}

func (f *fakeHTTP) Do(request *http.Request) (*http.Response, error) {
	body, ok := f.responses[request.URL.String()]
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(body))}, nil
}

func TestLatestUsesGitHubHeadersAndDecodesRelease(t *testing.T) {
	httpClient := &fakeHTTP{responses: map[string]string{"https://api.example/releases/latest": `{"tag_name":"v1.2.3","assets":[]}`}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example"}
	release, err := client.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v1.2.3" {
		t.Fatalf("tag = %q", release.TagName)
	}
}

func TestLatestReportsNoStableReleaseOn404(t *testing.T) {
	client := &Client{httpClient: &fakeHTTP{responses: map[string]string{}}, apiBase: "https://api.example"}
	_, err := client.Latest(context.Background())
	if !errors.Is(err, ErrNoStableRelease) {
		t.Fatalf("err = %v, want no-stable-release", err)
	}
}

func TestInstallVerifiesChecksumAndReplacesExecutable(t *testing.T) {
	archive := testArchive(t, "magelift", []byte("new binary"))
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:]) + "  magelift_1.2.3_linux_amd64.tar.gz\n"
	httpClient := &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           checksum,
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example", goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}, selfCheck: func(context.Context, string) error { return nil }}
	executable := filepath.Join(t.TempDir(), "magelift")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, executable)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new binary" {
		t.Fatalf("installed executable = %q", contents)
	}
	if _, err := os.Stat(executable + backupSuffix); !os.IsNotExist(err) {
		t.Fatalf("backup was not cleaned up: %v", err)
	}
}

func TestInstallRestoresPreviousBinaryOnSelfCheckFailure(t *testing.T) {
	archive := testArchive(t, "magelift", []byte("new binary"))
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:]) + "  magelift_1.2.3_linux_amd64.tar.gz\n"
	httpClient := &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           checksum,
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example", goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}, selfCheck: func(context.Context, string) error { return errors.New("exit status 1") }}
	executable := filepath.Join(t.TempDir(), "magelift")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, executable)
	if err == nil || !strings.Contains(err.Error(), "previous version restored") {
		t.Fatalf("err = %v, want restore", err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "old binary" {
		t.Fatalf("restored executable = %q", contents)
	}
}

func TestInstallReplacesStaleBackup(t *testing.T) {
	archive := testArchive(t, "magelift", []byte("new binary"))
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:]) + "  magelift_1.2.3_linux_amd64.tar.gz\n"
	httpClient := &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           checksum,
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example", goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}, selfCheck: func(context.Context, string) error { return nil }}
	dir := t.TempDir()
	executable := filepath.Join(dir, "magelift")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable+backupSuffix, []byte("stale junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, executable); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executable + backupSuffix); !os.IsNotExist(err) {
		t.Fatalf("stale backup survived: %v", err)
	}
}

func TestInstallSupportsFreshExecutable(t *testing.T) {
	archive := testArchive(t, "magelift", []byte("new binary"))
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:]) + "  magelift_1.2.3_linux_amd64.tar.gz\n"
	httpClient := &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           checksum,
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example", goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}, selfCheck: func(context.Context, string) error { return nil }}
	executable := filepath.Join(t.TempDir(), "magelift")
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, executable); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil || string(contents) != "new binary" {
		t.Fatalf("installed executable = %q, err = %v", contents, err)
	}
}

func TestInstallDefaultSelfCheckRunsVersion(t *testing.T) {
	script := "#!/bin/sh\nif [ \"$1\" = \"version\" ]; then echo magelift-test; exit 0; fi\nexit 1\n"
	archive := testArchive(t, "magelift", []byte(script))
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:]) + "  magelift_1.2.3_linux_amd64.tar.gz\n"
	httpClient := &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           checksum,
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example", goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}}
	executable := filepath.Join(t.TempDir(), "magelift")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No selfCheck stub: the default executes the installed script.
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, executable); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRejectsChecksumMismatch(t *testing.T) {
	archive := testArchive(t, "magelift", []byte("new binary"))
	client := &Client{httpClient: &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           "0000000000000000000000000000000000000000000000000000000000000000  magelift_1.2.3_linux_amd64.tar.gz\n",
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}, goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}}
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, filepath.Join(t.TempDir(), "magelift")); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

func TestInstallRejectsUnsignedRelease(t *testing.T) {
	client := &Client{httpClient: &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt": "" + hex.EncodeToString(make([]byte, 32)) + "  magelift_1.2.3_linux_amd64.tar.gz\n",
	}}, goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{}}
	assets := releaseAssets()[:2]
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: assets}, filepath.Join(t.TempDir(), "magelift")); err == nil {
		t.Fatal("unsigned release was accepted")
	}
}

func TestInstallRejectsInvalidReleaseSignature(t *testing.T) {
	archive := testArchive(t, "magelift", []byte("new binary"))
	digest := sha256.Sum256(archive)
	client := &Client{httpClient: &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           hex.EncodeToString(digest[:]) + "  magelift_1.2.3_linux_amd64.tar.gz\n",
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.tar.gz":         string(archive),
	}}, goos: "linux", goarch: "amd64", verifier: fakeBlobVerifier{err: errors.New("bad signature")}}
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: releaseAssets()}, filepath.Join(t.TempDir(), "magelift")); !errors.Is(err, ErrReleaseSignature) {
		t.Fatalf("error = %v", err)
	}
}

func releaseAssets() []Asset {
	return []Asset{
		{Name: "magelift_1.2.3_linux_amd64.tar.gz", DownloadURL: "https://api.example/magelift.tar.gz"},
		{Name: "checksums.txt", DownloadURL: "https://api.example/checksums.txt"},
		{Name: "checksums.txt.sigstore.json", DownloadURL: "https://api.example/checksums.sigstore.json"},
	}
}

func testArchive(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestInstallExtractsWindowsZip(t *testing.T) {
	archive := testZipArchive(t, "magelift.exe", []byte("new windows binary"))
	digest := sha256.Sum256(archive)
	checksum := hex.EncodeToString(digest[:]) + "  magelift_1.2.3_windows_amd64.zip\n"
	httpClient := &fakeHTTP{responses: map[string]string{
		"https://api.example/checksums.txt":           checksum,
		"https://api.example/checksums.sigstore.json": "{}",
		"https://api.example/magelift.zip":            string(archive),
	}}
	client := &Client{httpClient: httpClient, apiBase: "https://api.example", goos: "windows", goarch: "amd64", verifier: fakeBlobVerifier{}, selfCheck: func(context.Context, string) error { return nil }}
	executable := filepath.Join(t.TempDir(), "magelift.exe")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	assets := []Asset{
		{Name: "magelift_1.2.3_windows_amd64.zip", DownloadURL: "https://api.example/magelift.zip"},
		{Name: "checksums.txt", DownloadURL: "https://api.example/checksums.txt"},
		{Name: "checksums.txt.sigstore.json", DownloadURL: "https://api.example/checksums.sigstore.json"},
	}
	if err := client.Install(context.Background(), Release{TagName: "v1.2.3", Assets: assets}, executable); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new windows binary" {
		t.Fatalf("installed executable = %q", contents)
	}
}

func testZipArchive(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	zipWriter := zip.NewWriter(&output)
	entry, err := zipWriter.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
