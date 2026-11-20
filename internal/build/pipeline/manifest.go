package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	"github.com/magelift/magelift/internal/cosign"
)

type manifestImage struct {
	Digest    string `json:"imageDigest"`
	Reference string `json:"imageDigestReference"`
}

// DigestReference combines a build's registry repository and immutable digest.
func DigestReference(imageReference, digest string) (string, error) {
	name, _, _ := strings.Cut(imageReference, "@")
	lastSlash := strings.LastIndexByte(name, '/')
	if tag := strings.LastIndexByte(name, ':'); tag > lastSlash {
		name = name[:tag]
	}
	reference := name + "@" + digest
	if err := cosign.ValidateReference(reference); err != nil {
		return "", err
	}
	return reference, nil
}

// The host adds the push destination after verifying the builder's manifest.
func publishPushedManifest(path string, contents []byte, image buildkit.Result) (string, string, error) {
	reference, err := DigestReference(image.ImageReference, image.Digest)
	if err != nil {
		return "", "", err
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(contents, &manifest); err != nil || manifest == nil {
		return "", "", errors.New("the builder manifest must be a JSON object")
	}
	var digest string
	if err := json.Unmarshal(manifest["imageDigest"], &digest); err != nil || digest != image.Digest {
		return "", "", errors.New("the manifest image digest differs from the pushed image digest")
	}
	manifest["imageDigestReference"], err = json.Marshal(reference)
	if err != nil {
		return "", "", err
	}
	contents, err = json.Marshal(manifest)
	if err != nil {
		return "", "", err
	}
	if int64(len(contents)) > maxManifestBytes {
		return "", "", errors.New("the pushed build manifest exceeds 4 MiB")
	}

	// Concurrent builds of the same revision can push to different registries.
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	file, err := os.CreateTemp(filepath.Dir(path), name+"-*.json")
	if err != nil {
		return "", "", err
	}
	_, writeErr := file.Write(contents)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		_ = os.Remove(file.Name())
		return "", "", err
	}
	sum := sha256.Sum256(contents)
	return file.Name(), hex.EncodeToString(sum[:]), nil
}

// DeploymentDigest reads the registry-qualified digest from a pushed build manifest.
func DeploymentDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("open build manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return "", errors.New("the build manifest must be a regular file up to 4 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open build manifest: %w", err)
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return "", fmt.Errorf("read build manifest: %w", err)
	}
	if int64(len(contents)) > maxManifestBytes {
		return "", errors.New("the build manifest exceeds 4 MiB")
	}
	var image manifestImage
	if err := json.Unmarshal(contents, &image); err != nil {
		return "", fmt.Errorf("read build manifest JSON: %w", err)
	}
	if image.Reference == "" {
		return "", errors.New("the manifest has no registry digest. Set --digest for a previously pushed image or run magelift build --push for a new manifest")
	}
	if err := cosign.ValidateReference(image.Reference); err != nil {
		return "", fmt.Errorf("the build manifest contains an invalid registry digest: %w", err)
	}
	_, digest, _ := strings.Cut(image.Reference, "@")
	if digest != image.Digest {
		return "", errors.New("the manifest image digest differs from its registry digest")
	}
	return image.Reference, nil
}
