package certification

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var immutableImageDigest = regexp.MustCompile(`^[a-z0-9._-]+(?:/[a-z0-9._-]+)+@sha256:[0-9a-f]{64}$`)

// FailedDeploymentRequest is the immutable-artifact failed-deployment drill:
// apply a different digest, require the runtime to be unhealthy, then restore
// the current digest. Magento health or kube Ready are the caller's Healthy
// implementation; a successful control-plane apply is not proof.
type FailedDeploymentRequest struct {
	CurrentDigest string
	FaultDigest   string
}

// FailedDeploymentRuntime applies an immutable image digest and reports
// whether the Magento runtime is healthy afterward.
type FailedDeploymentRuntime interface {
	ApplyDigest(ctx context.Context, digest string) error
	Healthy(ctx context.Context) (bool, error)
}

// ValidateFailedDeploymentRequest refuses credential-like, mutable, or
// identical digests before any runtime mutation.
func ValidateFailedDeploymentRequest(request FailedDeploymentRequest) error {
	if strings.ContainsAny(request.CurrentDigest+request.FaultDigest, "\r\n\x00") {
		return errors.New("failed-deployment digest is required and must be single-line")
	}
	current := strings.TrimSpace(request.CurrentDigest)
	fault := strings.TrimSpace(request.FaultDigest)
	if err := validateImmutableImageDigest("current", current); err != nil {
		return err
	}
	if err := validateImmutableImageDigest("fault", fault); err != nil {
		return err
	}
	if current == fault {
		return errors.New("failed-deployment fault digest must differ from the current digest")
	}
	return nil
}

// InjectFailedDeployment applies the fault digest and requires the runtime to
// be unhealthy. ApplyDigest may return an error when Magento health fails
// closed; that is still a verified injection when Healthy is false.
func InjectFailedDeployment(ctx context.Context, runtime FailedDeploymentRuntime, request FailedDeploymentRequest) (FailureInjection, error) {
	if err := ValidateFailedDeploymentRequest(request); err != nil {
		return FailureInjection{}, err
	}
	if ctx == nil {
		return FailureInjection{}, errors.New("failed-deployment context is required")
	}
	if err := ctx.Err(); err != nil {
		return FailureInjection{}, err
	}
	if runtime == nil {
		return FailureInjection{}, errors.New("failed-deployment runtime is required")
	}
	applyErr := runtime.ApplyDigest(ctx, strings.TrimSpace(request.FaultDigest))
	healthy, healthErr := runtime.Healthy(ctx)
	if healthErr != nil {
		return FailureInjection{}, errors.Join(applyErr, fmt.Errorf("observe failed-deployment health: %w", healthErr))
	}
	if healthy {
		if applyErr != nil {
			return FailureInjection{}, fmt.Errorf("fault digest remained healthy after apply error: %w", applyErr)
		}
		return FailureInjection{}, errors.New("fault digest became healthy; failed-deployment was not injected")
	}
	return FailureInjection{
		ID:           "failed-deployment",
		ResourceRefs: []string{"immutable-artifact"},
		Verified:     true,
	}, nil
}

// RestoreFailedDeployment reapplies the current digest and requires the
// runtime to become healthy. It is independent of InjectFailedDeployment
// success so a caller can recover after a verified fault.
func RestoreFailedDeployment(ctx context.Context, runtime FailedDeploymentRuntime, request FailedDeploymentRequest) error {
	if err := ValidateFailedDeploymentRequest(request); err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("failed-deployment context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime == nil {
		return errors.New("failed-deployment runtime is required")
	}
	if err := runtime.ApplyDigest(ctx, strings.TrimSpace(request.CurrentDigest)); err != nil {
		return fmt.Errorf("restore current digest: %w", err)
	}
	healthy, err := runtime.Healthy(ctx)
	if err != nil {
		return fmt.Errorf("observe restored failed-deployment health: %w", err)
	}
	if !healthy {
		return errors.New("current digest did not become healthy after failed-deployment restore")
	}
	return nil
}

func validateImmutableImageDigest(label, digest string) error {
	if digest == "" || strings.ContainsAny(digest, "\r\n\x00") {
		return fmt.Errorf("failed-deployment %s digest is required and must be single-line", label)
	}
	if strings.Contains(digest, "://") || strings.Count(digest, "@") > 1 {
		return fmt.Errorf("failed-deployment %s digest must not contain credentials", label)
	}
	if !immutableImageDigest.MatchString(digest) {
		return fmt.Errorf("failed-deployment %s digest must be an immutable repository@sha256 digest", label)
	}
	return nil
}
