package certification

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
	"github.com/magelift/magelift/sdk"
)

// PipelineBuildFunc runs the existing isolated application build. It is
// called only by the owner of an acquired compatibility boundary.
type PipelineBuildFunc func(context.Context) (buildpipeline.Result, error)

// ArtifactSigner is the provider-neutral signing boundary. Implementations
// resolve their own credentials and must never return secret material in an
// error. The reference is always the fully-qualified immutable image digest,
// never a tag.
type ArtifactSigner interface {
	Sign(context.Context, string) error
}

// ArtifactVerifier is the provider-neutral signature verification boundary.
// Certificate identity and OIDC issuer are policy values, not credentials;
// implementations must still redact any provider error before returning it.
// The method shape intentionally uses scalar values so an implementation such
// as cosign can satisfy the interface without exposing its SDK types here.
type ArtifactVerifier interface {
	VerifyArtifact(context.Context, string, string, string) error
}

// ArtifactSigningOptions contains the non-secret identities recorded with an
// immutable artifact and the injected signing and verification operations.
// SignatureReference is supplied by the caller because the certification core
// does not own a registry or invent a publishing API.
type ArtifactSigningOptions struct {
	ProvenanceReference string
	SignatureReference  string
	CertificateIdentity string
	OIDCIssuer          string
	Signer              ArtifactSigner
	Verifier            ArtifactVerifier
}

func (options ArtifactSigningOptions) Validate() error {
	if err := options.validateMetadata(false); err != nil {
		return err
	}
	if isNilArtifactDependency(options.Signer) {
		return errors.New("artifact signer is required")
	}
	if isNilArtifactDependency(options.Verifier) {
		return errors.New("artifact verifier is required")
	}
	return nil
}

func (options ArtifactSigningOptions) validateMetadata(allowEmpty bool) error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "provenance reference", value: options.ProvenanceReference},
		{name: "signature reference", value: options.SignatureReference},
		{name: "certificate identity", value: options.CertificateIdentity},
		{name: "certificate OIDC issuer", value: options.OIDCIssuer},
	}
	if allowEmpty {
		empty := true
		for _, field := range fields {
			if field.value != "" {
				empty = false
				break
			}
		}
		if empty {
			return nil
		}
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" || strings.ContainsAny(field.value, "\r\n\x00") {
			return fmt.Errorf("artifact %s is required and must be single-line", field.name)
		}
		if err := ValidateSecretSafeText(field.value); err != nil {
			return fmt.Errorf("artifact %s contains secret material", field.name)
		}
	}
	return nil
}

func isNilArtifactDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type safeArtifactError struct {
	operation string
	cause     error
}

func (err safeArtifactError) Error() string {
	if err.cause == nil {
		return err.operation
	}
	message, _ := RedactSensitiveText(err.cause.Error())
	if strings.TrimSpace(message) == "" {
		return err.operation
	}
	return fmt.Sprintf("%s: %s", err.operation, message)
}

func (err safeArtifactError) Unwrap() error {
	return err.cause
}

func safeArtifactFailure(operation string, cause error) error {
	if cause == nil {
		return nil
	}
	return safeArtifactError{operation: operation, cause: cause}
}

// canonicalizePipelineImageDigest removes a mutable image tag that may be
// retained by the pipeline result before its separately reported digest. The
// certification and cosign boundaries accept only repository@digest, so the
// digest supplied by the pipeline remains the sole content identity.
func canonicalizePipelineImageDigest(contract sdk.ImmutableArtifactContract) (sdk.ImmutableArtifactContract, error) {
	name, digest, found := strings.Cut(contract.ImageDigest, "@")
	if !found {
		return sdk.ImmutableArtifactContract{}, errors.New("immutable certification artifact image digest is malformed")
	}
	lastSlash := strings.LastIndexByte(name, '/')
	if lastColon := strings.LastIndexByte(name, ':'); lastColon > lastSlash {
		name = name[:lastColon]
	}
	contract.ImageDigest = name + "@" + digest
	if err := contract.Validate(); err != nil {
		return sdk.ImmutableArtifactContract{}, err
	}
	return contract, nil
}

// EnsurePipelineArtifact connects the build pipeline's immutable result to
// the certification registry. The registry decides whether the build runs;
// this helper signs and verifies the immutable image digest only on a cache
// miss, then maps the pipeline output to the public artifact contract.
func EnsurePipelineArtifact(ctx context.Context, registry ImmutableArtifactRegistry, request ArtifactBuildRequest, build PipelineBuildFunc, options ArtifactSigningOptions) (ImmutableArtifactRecord, bool, error) {
	if err := options.validateMetadata(true); err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	return EnsureImmutableArtifact(ctx, registry, request, func(ctx context.Context, request ArtifactBuildRequest) (contract sdk.ImmutableArtifactContract, err error) {
		if build == nil {
			return sdk.ImmutableArtifactContract{}, errors.New("pipeline artifact builder is required")
		}
		if err := options.Validate(); err != nil {
			return sdk.ImmutableArtifactContract{}, err
		}
		result, err := build(ctx)
		if err != nil {
			return sdk.ImmutableArtifactContract{}, safeArtifactFailure("build immutable certification artifact", err)
		}
		contract, err = result.ImmutableArtifactContract(request.CompatibilityFingerprint, options.ProvenanceReference, options.SignatureReference)
		if err != nil {
			return sdk.ImmutableArtifactContract{}, safeArtifactFailure("create immutable certification artifact contract", err)
		}
		contract, err = canonicalizePipelineImageDigest(contract)
		if err != nil {
			return sdk.ImmutableArtifactContract{}, safeArtifactFailure("canonicalize immutable certification artifact digest", err)
		}
		if err := ValidateSecretSafeValue(contract); err != nil {
			return sdk.ImmutableArtifactContract{}, errors.New("immutable certification artifact contract contains secret material")
		}
		if err := options.Signer.Sign(ctx, contract.ImageDigest); err != nil {
			return sdk.ImmutableArtifactContract{}, safeArtifactFailure("sign immutable certification artifact", err)
		}
		if err := options.Verifier.VerifyArtifact(ctx, contract.ImageDigest, options.CertificateIdentity, options.OIDCIssuer); err != nil {
			return sdk.ImmutableArtifactContract{}, safeArtifactFailure("verify immutable certification artifact", err)
		}
		return contract, nil
	})
}
