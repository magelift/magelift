package providerhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// generatorDownloadBase serves release assets. Generated entries carry
	// absolute binary and bundle URLs so providers install can fetch them
	// with no beside-CLI bundle.
	generatorDownloadBase = "https://github.com/magelift/magelift/releases/download/"
)

// GenerateLockfiles builds one lockfile per platform from a GoReleaser
// dist directory: provider binary digests from checksums.txt, bundle paths
// from artifacts.json. Every emitted entry passes the loader's own
// validation, so a generated lockfile always loads. A dist tree without
// provider artifacts is a broken release and fails.
func GenerateLockfiles(distDir, tag string) (map[string]Lockfile, error) {
	if strings.TrimSpace(tag) == "" {
		return nil, errors.New("release tag is required")
	}
	checksums, err := readChecksums(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		return nil, err
	}
	signatures, err := readSignaturePaths(filepath.Join(distDir, "artifacts.json"))
	if err != nil {
		return nil, err
	}
	root := distRoot(distDir)
	entries := map[string]map[string]Artifact{}
	for _, asset := range sortedKeys(checksums) {
		if !strings.HasPrefix(asset, "magelift-provider-") {
			continue
		}
		if strings.HasSuffix(asset, ".sigstore.json") || strings.HasSuffix(asset, ".sbom.json") {
			continue
		}
		name, platform, err := splitProviderAsset(asset)
		if err != nil {
			return nil, err
		}
		bundle := asset + ".sigstore.json"
		bundlePath, ok := signatures[bundle]
		if !ok || !fileExists(filepath.Join(root, bundlePath)) {
			return nil, fmt.Errorf("missing bundle for %s", asset)
		}
		artifact := Artifact{
			Name:     "magelift-provider-" + name,
			Version:  strings.TrimSpace(tag),
			Protocol: ProtocolV1Marker,
			Digest:   "sha256:" + checksums[asset],
			URL:      generatorDownloadBase + strings.TrimSpace(tag) + "/" + asset,
			Cosign: CosignTrust{
				Identity: FirstPartyIdentityPrefix + strings.TrimSpace(tag),
				Issuer:   FirstPartyIssuer,
				Bundle:   generatorDownloadBase + strings.TrimSpace(tag) + "/" + bundle,
			},
		}
		if err := artifact.validate(); err != nil {
			return nil, fmt.Errorf("generated entry for %s: %w", asset, err)
		}
		if entries[platform] == nil {
			entries[platform] = map[string]Artifact{}
		}
		entries[platform][name] = artifact
	}
	if len(entries) == 0 {
		return nil, errors.New("no provider artifacts in checksums.txt")
	}
	locks := make(map[string]Lockfile, len(entries))
	for platform, providers := range entries {
		locks[platform] = Lockfile{SchemaVersion: SchemaVersion, SDKAPIVersion: SDKAPIVersion, Providers: providers}
	}
	return locks, nil
}

// splitProviderAsset parses magelift-provider-<name>_<version>_<os>_<arch>[.exe].
func splitProviderAsset(asset string) (name, platform string, err error) {
	rest := strings.TrimPrefix(asset, "magelift-provider-")
	name, tail, ok := strings.Cut(rest, "_")
	if !ok || name == "" {
		return "", "", fmt.Errorf("provider asset %q has no name/version separator", asset)
	}
	_, platform, ok = strings.Cut(tail, "_")
	if !ok || platform == "" {
		return "", "", fmt.Errorf("provider asset %q has no version/platform separator", asset)
	}
	platform = strings.TrimSuffix(platform, ".exe")
	return name, platform, nil
}

func distRoot(distDir string) string {
	absolute, err := filepath.Abs(distDir)
	if err != nil {
		return "."
	}
	return filepath.Dir(absolute)
}

func readChecksums(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checksums.txt: %w", err)
	}
	checksums := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		checksums[parts[1]] = parts[0]
	}
	return checksums, nil
}

func readSignaturePaths(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read artifacts.json: %w", err)
	}
	var artifacts []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &artifacts); err != nil {
		return nil, fmt.Errorf("decode artifacts.json: %w", err)
	}
	paths := map[string]string{}
	for _, artifact := range artifacts {
		if artifact.Type == "Signature" {
			paths[artifact.Name] = artifact.Path
		}
	}
	return paths, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
