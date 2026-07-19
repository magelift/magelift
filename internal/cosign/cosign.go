package cosign

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrInvalidReference = errors.New("image reference must be a registry-qualified sha256 digest")
	ErrInvalidIdentity  = errors.New("certificate identity is invalid")
	ErrInvalidIssuer    = errors.New("certificate OIDC issuer must be a valid HTTPS URL")
	ErrInvalidBlobPath  = errors.New("cosign blob paths are invalid")
	ErrRunnerRequired   = errors.New("cosign command runner is required")
	ErrSignFailed       = errors.New("cosign signing failed")
	ErrVerifyFailed     = errors.New("cosign verification failed")
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var registryPattern = regexp.MustCompile(`^(?:localhost|[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)(?::[1-9][0-9]{0,4})?$`)
var repositorySegmentPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*$`)

type CommandRunner interface {
	Run(context.Context, string, ...string) error
}

type Client struct {
	runner CommandRunner
}

type VerifyOptions struct {
	CertificateIdentity string
	OIDCIssuer          string
}

func New() *Client {
	return &Client{runner: execRunner{}}
}

func NewWithRunner(runner CommandRunner) *Client {
	return &Client{runner: runner}
}

func ValidateReference(reference string) error {
	return validateReference(reference)
}

func (c *Client) Sign(ctx context.Context, reference string) error {
	if err := validateReference(reference); err != nil {
		return err
	}
	if c == nil || c.runner == nil {
		return ErrRunnerRequired
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if err := c.runner.Run(ctx, "cosign", "sign", "--yes", reference); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return ErrSignFailed
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return nil
}

func (c *Client) Verify(ctx context.Context, reference string, options VerifyOptions) error {
	if err := validateReference(reference); err != nil {
		return err
	}
	if options.CertificateIdentity == "" || strings.IndexFunc(options.CertificateIdentity, unicode.IsControl) >= 0 {
		return ErrInvalidIdentity
	}
	if err := validateIssuer(options.OIDCIssuer); err != nil {
		return err
	}
	if c == nil || c.runner == nil {
		return ErrRunnerRequired
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if err := c.runner.Run(ctx,
		"cosign", "verify",
		"--certificate-identity", options.CertificateIdentity,
		"--certificate-oidc-issuer", options.OIDCIssuer,
		reference,
	); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return ErrVerifyFailed
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return nil
}

func (c *Client) VerifyBlob(ctx context.Context, bundlePath, artifactPath string, options VerifyOptions) error {
	if !validBlobPath(bundlePath) || !validBlobPath(artifactPath) {
		return ErrInvalidBlobPath
	}
	if options.CertificateIdentity == "" || strings.IndexFunc(options.CertificateIdentity, unicode.IsControl) >= 0 {
		return ErrInvalidIdentity
	}
	if err := validateIssuer(options.OIDCIssuer); err != nil {
		return err
	}
	if c == nil || c.runner == nil {
		return ErrRunnerRequired
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if err := c.runner.Run(ctx,
		"cosign", "verify-blob",
		"--bundle", bundlePath,
		"--certificate-identity", options.CertificateIdentity,
		"--certificate-oidc-issuer", options.OIDCIssuer,
		artifactPath,
	); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return ErrVerifyFailed
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return nil
}

func validBlobPath(path string) bool {
	return path != "" && !strings.HasPrefix(path, "-") && strings.IndexFunc(path, unicode.IsControl) < 0
}

func validateReference(reference string) error {
	name, digest, found := strings.Cut(reference, "@")
	if !found || strings.Contains(digest, "@") || !digestPattern.MatchString(digest) {
		return ErrInvalidReference
	}
	registry, repository, found := strings.Cut(name, "/")
	qualifiedRegistry := registry == "localhost" || strings.ContainsAny(registry, ".:")
	if !found || !qualifiedRegistry || !validRegistry(registry) || repository == "" {
		return ErrInvalidReference
	}
	for _, segment := range strings.Split(repository, "/") {
		if !repositorySegmentPattern.MatchString(segment) {
			return ErrInvalidReference
		}
	}
	return nil
}

func validRegistry(registry string) bool {
	if !registryPattern.MatchString(registry) {
		return false
	}
	_, port, found := strings.Cut(registry, ":")
	if !found {
		return true
	}
	number, err := strconv.ParseUint(port, 10, 16)
	return err == nil && number != 0
}

func validateIssuer(rawURL string) error {
	issuer, err := url.Parse(rawURL)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return ErrInvalidIssuer
	}
	return nil
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}
