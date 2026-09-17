// Package upgrade implements the checksum-verified release update used by the
// MageLift CLI. Network and filesystem work stay behind small interfaces so
// the command is testable without GitHub or a second binary.
package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cosign"
)

const defaultAPI = "https://api.github.com/repos/magelift/magelift"

var (
	ErrNoReleaseAsset   = errors.New("release does not contain a compatible MageLift archive")
	ErrChecksum         = errors.New("release checksum verification failed")
	ErrReleaseSignature = errors.New("release checksum signature verification failed")
	ErrNoStableRelease  = errors.New("no stable release published yet; install a specific tag with --version")
	errReleaseNotFound  = errors.New("release not found")
)

var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type blobVerifier interface {
	VerifyBlob(context.Context, string, string, cosign.VerifyOptions) error
}

type Client struct {
	httpClient HTTPClient
	apiBase    string
	goos       string
	goarch     string
	verifier   blobVerifier
	// selfCheck runs the installed binary after the swap; nil uses the
	// default `version` execution. Tests stub it to avoid executing
	// fixture bytes.
	selfCheck func(context.Context, string) error
	// files performs the swap transitions; nil uses the OS. Tests
	// script failures at each transition.
	files fileOps
}

// fileOps covers the filesystem transitions of the swap so fault tests
// can fail each one. Production uses osFileOps.
type fileOps interface {
	Copy(src, dst string) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

type osFileOps struct{}

func (osFileOps) Copy(src, dst string) error { return copyFile(src, dst) }
func (osFileOps) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (osFileOps) Remove(name string) error { return os.Remove(name) }

func (c *Client) fileOps() fileOps {
	if c != nil && c.files != nil {
		return c.files
	}
	return osFileOps{}
}

// copyFile duplicates src to dst with its permission bits. A failed copy
// removes the partial destination; a crash may still leave one, which the
// swap tolerates (backups are replaced, never executed).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open copy source: %w", err)
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat copy source: %w", err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create copy destination: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		os.Remove(dst)
		return fmt.Errorf("copy file contents: %v %v %v", copyErr, syncErr, closeErr)
	}
	return nil
}

// backupSuffix names the previous binary kept beside the executable until
// the replacement proves it runs. Same directory, so the swap and any
// restore stay on one filesystem.
const backupSuffix = ".prev"

const selfCheckTimeout = 30 * time.Second

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func New(httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("MAGELIFT_UPDATE_API_URL")), "/")
	if base == "" {
		base = defaultAPI
	}
	return &Client{httpClient: httpClient, apiBase: base, goos: runtime.GOOS, goarch: runtime.GOARCH, verifier: cosign.New()}
}

// Latest resolves the stable channel (/releases/latest skips drafts and
// prereleases). Before the first stable release it fails cleanly with
// ErrNoStableRelease; prereleases install via Tagged with an explicit tag.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	release, err := c.release(ctx, c.apiBase+"/releases/latest")
	if err != nil {
		if errors.Is(err, errReleaseNotFound) {
			return Release{}, ErrNoStableRelease
		}
		return Release{}, err
	}
	return release, nil
}

func (c *Client) Tagged(ctx context.Context, tag string) (Release, error) {
	if strings.TrimSpace(tag) == "" {
		return Release{}, errors.New("release tag is required")
	}
	return c.release(ctx, c.apiBase+"/releases/tags/"+url.PathEscape(tag))
}

func (c *Client) release(ctx context.Context, endpoint string) (Release, error) {
	if c == nil || c.httpClient == nil {
		return Release{}, errors.New("upgrade client is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, fmt.Errorf("create release request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "magelift-upgrade")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("fetch MageLift release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Release{}, errReleaseNotFound
	}
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetch MageLift release: HTTP %s", response.Status)
	}
	var release Release
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("decode MageLift release: %w", err)
	}
	if release.TagName == "" {
		return Release{}, errors.New("GitHub release has no tag")
	}
	if !releaseTagPattern.MatchString(release.TagName) {
		return Release{}, fmt.Errorf("GitHub release tag %q is not a semantic version", release.TagName)
	}
	return release, nil
}

func (c *Client) Install(ctx context.Context, release Release, executable string) error {
	if strings.TrimSpace(executable) == "" {
		return errors.New("current executable path is required")
	}
	archive, checksum, signature, expectedChecksum, err := c.assets(ctx, release)
	if err != nil {
		return err
	}
	checksumBody, err := c.download(ctx, checksum.DownloadURL)
	if err != nil {
		return err
	}
	signatureBody, err := c.download(ctx, signature.DownloadURL)
	if err != nil {
		return err
	}
	if c.verifier == nil {
		return ErrReleaseSignature
	}
	checksumPath, err := writeTemporaryAsset("magelift-checksums-*", checksumBody)
	if err != nil {
		return err
	}
	defer os.Remove(checksumPath)
	signaturePath, err := writeTemporaryAsset("magelift-checksums-*.sigstore.json", signatureBody)
	if err != nil {
		return err
	}
	defer os.Remove(signaturePath)
	if err := c.verifier.VerifyBlob(ctx, signaturePath, checksumPath, cosign.VerifyOptions{
		CertificateIdentity: releaseIdentity(release.TagName),
		OIDCIssuer:          "https://token.actions.githubusercontent.com",
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrReleaseSignature, err)
	}
	archiveBody, err := c.download(ctx, archive.DownloadURL)
	if err != nil {
		return err
	}
	if err := verifyChecksum(archiveBody, expectedChecksum); err != nil {
		return err
	}
	binary, err := extractBinary(archiveBody, archive.Name, filepath.Base(executable))
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(executable), ".magelift-upgrade-*")
	if err != nil {
		return fmt.Errorf("create upgrade file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o755); err != nil {
		temporary.Close()
		return fmt.Errorf("set upgrade permissions: %w", err)
	}
	if _, err := temporary.Write(binary); err != nil {
		temporary.Close()
		return fmt.Errorf("write upgrade file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync upgrade file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close upgrade file: %w", err)
	}
	ops := c.fileOps()
	backup := executable + backupSuffix
	// A stale backup from a killed run is replaced, never executed.
	if err := ops.Remove(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear stale upgrade backup: %w", err)
	}
	// Crash safety: the backup is a COPY, so the executable never leaves
	// its path until the single atomic replacement rename. A kill before
	// the rename leaves the old binary (plus maybe a partial .prev, which
	// is replaced, never executed); a kill during rename leaves old or
	// new; the command is never missing. Windows cannot replace a running
	// executable: the rename fails and the old binary stays (documented
	// in docs/install.md).
	_, statErr := os.Stat(executable)
	switch {
	case statErr == nil:
		if err := ops.Copy(executable, backup); err != nil {
			return fmt.Errorf("back up current executable: %w", err)
		}
		if err := ops.Rename(temporaryName, executable); err != nil {
			return fmt.Errorf("replace current executable failed; previous version untouched (spare copy at %s): %w", backup, err)
		}
	case os.IsNotExist(statErr):
		// Fresh install: no previous binary to keep.
		backup = ""
		if err := ops.Rename(temporaryName, executable); err != nil {
			return fmt.Errorf("replace current executable: %w", err)
		}
	default:
		return fmt.Errorf("stat current executable: %w", statErr)
	}
	if err := c.checkInstalled(ctx, executable); err != nil {
		if backup == "" {
			ops.Remove(executable)
			return fmt.Errorf("new binary failed its self-check: %w", err)
		}
		if restoreErr := ops.Rename(backup, executable); restoreErr != nil {
			return fmt.Errorf("new binary failed its self-check: %v (restore also failed: %v; previous binary kept at %s)", err, restoreErr, backup)
		}
		return fmt.Errorf("new binary failed its self-check, previous version restored: %w", err)
	}
	// The upgrade succeeded; a leftover backup is inert litter the next
	// run replaces. Never fail a good install over its cleanup.
	if backup != "" {
		_ = ops.Remove(backup)
	}
	return nil
}

func (c *Client) checkInstalled(ctx context.Context, executable string) error {
	check := defaultSelfCheck
	if c != nil && c.selfCheck != nil {
		check = c.selfCheck
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, selfCheckTimeout)
	defer cancel()
	return check(ctx, executable)
}

func defaultSelfCheck(ctx context.Context, executable string) error {
	command := exec.CommandContext(ctx, executable, "version")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("run %s version: %v: %s", executable, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (c *Client) assets(ctx context.Context, release Release) (Asset, Asset, Asset, string, error) {
	if !releaseTagPattern.MatchString(release.TagName) {
		return Asset{}, Asset{}, Asset{}, "", fmt.Errorf("release tag %q is not a semantic version", release.TagName)
	}
	archiveName := archiveNameFor(c.goos, strings.TrimPrefix(release.TagName, "v"), c.goarch)
	var archive, checksums, signature Asset
	for _, asset := range release.Assets {
		switch asset.Name {
		case archiveName:
			archive = asset
		case "checksums.txt":
			checksums = asset
		case "checksums.txt.sigstore.json":
			signature = asset
		}
	}
	if archive.DownloadURL == "" || checksums.DownloadURL == "" || signature.DownloadURL == "" {
		return Asset{}, Asset{}, Asset{}, "", ErrNoReleaseAsset
	}
	if err := validateDownloadURL(archive.DownloadURL); err != nil {
		return Asset{}, Asset{}, Asset{}, "", err
	}
	if err := validateDownloadURL(checksums.DownloadURL); err != nil {
		return Asset{}, Asset{}, Asset{}, "", err
	}
	if err := validateDownloadURL(signature.DownloadURL); err != nil {
		return Asset{}, Asset{}, Asset{}, "", err
	}
	body, err := c.download(ctx, checksums.DownloadURL)
	if err != nil {
		return Asset{}, Asset{}, Asset{}, "", err
	}
	expected, err := checksumForArchive(body, archive.Name)
	if err != nil {
		return Asset{}, Asset{}, Asset{}, "", err
	}
	return archive, checksums, signature, expected, nil
}

// archiveNameFor mirrors the GoReleaser archives matrix: the cli archive
// uses name_template magelift_<version>_<os>_<arch> with tar.gz everywhere
// except Windows, which overrides to zip. Version arrives without the tag's
// leading v.
func archiveNameFor(goos, version, goarch string) string {
	if goos == "windows" {
		return fmt.Sprintf("magelift_%s_%s_%s.zip", version, goos, goarch)
	}
	return fmt.Sprintf("magelift_%s_%s_%s.tar.gz", version, goos, goarch)
}

func (c *Client) download(ctx context.Context, endpoint string) ([]byte, error) {
	if err := validateDownloadURL(endpoint); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create asset request: %w", err)
	}
	request.Header.Set("User-Agent", "magelift-upgrade")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download release asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download release asset: HTTP %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256<<20))
	if err != nil {
		return nil, fmt.Errorf("read release asset: %w", err)
	}
	return body, nil
}

func validateDownloadURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("release asset URL must be an HTTPS URL without credentials or query parameters")
	}
	return nil
}

func verifyChecksum(body []byte, expected string) error {
	actual := sha256.Sum256(body)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), strings.TrimSpace(expected)) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksum, strings.TrimSpace(expected), hex.EncodeToString(actual[:]))
	}
	return nil
}

func checksumForArchive(body []byte, archiveName string) (string, error) {
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == archiveName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%w: checksum for %s is missing", ErrNoReleaseAsset, archiveName)
}

func releaseIdentity(tag string) string {
	return "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/" + tag
}

func writeTemporaryAsset(pattern string, body []byte) (string, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temporary release asset: %w", err)
	}
	name := file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if err = file.Chmod(0o600); err == nil {
		_, err = file.Write(body)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("write temporary release asset: %w", err)
	}
	return name, nil
}

func extractBinary(archive []byte, assetName, executableName string) ([]byte, error) {
	if executableName == "" {
		return nil, errors.New("executable name is required")
	}
	// Format follows the GoReleaser archives matrix: tar.gz everywhere,
	// zip on Windows (format_overrides). The asset name, already matched
	// against the release, selects the reader.
	if strings.HasSuffix(assetName, ".zip") {
		return extractZipBinary(archive, executableName)
	}
	return extractTarBinary(archive, executableName)
}

func extractZipBinary(archive []byte, executableName string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != executableName || strings.Contains(filepath.Clean(file.Name), "..") {
			continue
		}
		if file.UncompressedSize64 == 0 || file.UncompressedSize64 > 128<<20 {
			return nil, errors.New("release executable has an invalid size")
		}
		opened, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("read release executable: %w", err)
		}
		binary, err := io.ReadAll(io.LimitReader(opened, int64(file.UncompressedSize64)+1))
		closeErr := opened.Close()
		if err != nil {
			return nil, fmt.Errorf("read release executable: %w", err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("read release executable: %w", closeErr)
		}
		if uint64(len(binary)) != file.UncompressedSize64 {
			return nil, errors.New("release executable was truncated")
		}
		return binary, nil
	}
	return nil, ErrNoReleaseAsset
}

func extractTarBinary(archive []byte, executableName string) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != executableName || strings.Contains(filepath.Clean(header.Name), "..") {
			continue
		}
		if header.Size <= 0 || header.Size > 128<<20 {
			return nil, errors.New("release executable has an invalid size")
		}
		binary, err := io.ReadAll(io.LimitReader(tarReader, header.Size+1))
		if err != nil {
			return nil, fmt.Errorf("read release executable: %w", err)
		}
		if int64(len(binary)) != header.Size {
			return nil, errors.New("release executable was truncated")
		}
		return binary, nil
	}
	return nil, ErrNoReleaseAsset
}
