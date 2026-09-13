package providerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/magelift/magelift/internal/cosign"
)

type blobVerifier interface {
	VerifyBlob(context.Context, string, string, cosign.VerifyOptions) error
}

// VerifyLocal checks a downloaded subprocess binary against the lockfile
// digest and Cosign blob signature. It does not execute the file.
func VerifyLocal(ctx context.Context, artifact Artifact, binaryPath, bundlePath string, verifier blobVerifier) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := artifact.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(binaryPath) == "" {
		return errors.New("provider binary path is required")
	}
	if strings.TrimSpace(bundlePath) == "" {
		return fmt.Errorf("%w: cosign bundle path is required", ErrUnsigned)
	}
	if verifier == nil {
		return fmt.Errorf("%w: cosign verifier is required", ErrUnsigned)
	}
	sum, err := fileSHA256(binaryPath)
	if err != nil {
		return err
	}
	want := strings.TrimSpace(artifact.Digest)
	if "sha256:"+sum != want {
		return fmt.Errorf("%w: got sha256:%s want %s", ErrChecksumMismatch, sum, want)
	}
	if err := verifier.VerifyBlob(ctx, bundlePath, binaryPath, cosign.VerifyOptions{
		CertificateIdentity: artifact.Cosign.Identity,
		OIDCIssuer:          artifact.Cosign.Issuer,
	}); err != nil {
		return fmt.Errorf("verify provider signature: %w", err)
	}
	return nil
}

const maxProviderBytes = 1 << 30

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open provider artifact: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat provider artifact: %w", err)
	}
	if info.Size() > maxProviderBytes {
		return "", fmt.Errorf("provider artifact exceeds %d bytes", maxProviderBytes)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash provider artifact: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
