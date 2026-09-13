package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// IdentityCheckRequest describes the provider account and minimum permission
// set that an acceptance or lifecycle client must prove before mutation.
type IdentityCheckRequest struct {
	Provider            sdk.ProviderID
	AccountOrProjectRef string
	Region              string
	RequiredPermissions []string
}

// PermissionCheck is normalized evidence from the provider's identity API or
// access analyzer. It contains no policy document or credential material.
type PermissionCheck struct {
	Name    string
	Granted bool
}

// IdentityObservation is safe to return to admission and certification. A
// provider must set LeastPrivilegeVerified only when it has checked the
// requested scope against its native identity/permission mechanism.
type IdentityObservation struct {
	Provider               sdk.ProviderID
	AccountOrProjectRef    string
	Region                 string
	PrincipalRef           string
	PermissionChecks       []PermissionCheck
	LeastPrivilegeVerified bool
}

// IdentityClient is the provider-specific account and least-privilege seam.
// AWS, GCP, Scaleway, OVHcloud, and community providers can implement it
// without exposing their SDK identity types to the core.
type IdentityClient interface {
	CheckIdentity(context.Context, IdentityCheckRequest) (IdentityObservation, error)
}

// PermissionVerifier is implemented at the provider edge when the vendor
// exposes an access-analyzer or permission-test API. Returning false is
// deliberately conservative: account visibility alone is not proof of the
// mutation scope required by a certification run.
type PermissionVerifier interface {
	Verify(context.Context, sdk.ProviderID, string, string, []string) (bool, error)
}

// VerifyIdentity validates one provider observation before a paid mutation is
// admitted. Missing or unverified permissions are hard failures, not a
// best-effort warning.
func VerifyIdentity(ctx context.Context, client IdentityClient, request IdentityCheckRequest) (IdentityObservation, error) {
	if ctx == nil {
		return IdentityObservation{}, errors.New("identity context is required")
	}
	if client == nil {
		return IdentityObservation{}, errors.New("identity client is required")
	}
	if err := validateIdentityRequest(request); err != nil {
		return IdentityObservation{}, err
	}
	if err := ctx.Err(); err != nil {
		return IdentityObservation{}, err
	}
	observation, err := client.CheckIdentity(ctx, request)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return IdentityObservation{}, contextErr
		}
		return IdentityObservation{}, errors.New("provider identity check failed")
	}
	if err := validateIdentityObservation(request, observation); err != nil {
		return IdentityObservation{}, err
	}
	return observation, nil
}

func validateIdentityRequest(request IdentityCheckRequest) error {
	var problems []error
	if err := sdk.ValidateTargetDescriptor(sdk.TargetDescriptor{Provider: request.Provider, Runtime: "identity", ID: "identity"}); err != nil {
		problems = append(problems, err)
	}
	for name, value := range map[string]string{"account or project reference": request.AccountOrProjectRef, "region": request.Region} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("identity %s is required and must be single-line", name))
		}
	}
	seen := make(map[string]struct{}, len(request.RequiredPermissions))
	for _, permission := range request.RequiredPermissions {
		permission = strings.TrimSpace(permission)
		if permission == "" || strings.ContainsAny(permission, "\r\n\x00") {
			problems = append(problems, errors.New("identity permission names must be non-empty and single-line"))
		}
		if _, exists := seen[permission]; exists {
			problems = append(problems, fmt.Errorf("identity permission %q is duplicated", permission))
		}
		seen[permission] = struct{}{}
	}
	return errors.Join(problems...)
}

func validateIdentityObservation(request IdentityCheckRequest, observation IdentityObservation) error {
	if observation.Provider != request.Provider {
		return fmt.Errorf("provider identity %q does not match requested provider %q", observation.Provider, request.Provider)
	}
	if observation.AccountOrProjectRef != request.AccountOrProjectRef {
		return errors.New("provider identity does not match requested account or project")
	}
	if observation.Region != request.Region {
		return errors.New("provider identity does not match requested region")
	}
	if strings.TrimSpace(observation.PrincipalRef) == "" || strings.ContainsAny(observation.PrincipalRef, "\r\n\x00") {
		return errors.New("provider identity principal reference is required and must be single-line")
	}
	if !observation.LeastPrivilegeVerified {
		return errors.New("provider identity did not verify least-privilege scope")
	}
	checks := make(map[string]bool, len(observation.PermissionChecks))
	for _, check := range observation.PermissionChecks {
		if strings.TrimSpace(check.Name) == "" || strings.ContainsAny(check.Name, "\r\n\x00") {
			return errors.New("provider identity permission check name is required and must be single-line")
		}
		if _, exists := checks[check.Name]; exists {
			return fmt.Errorf("provider identity permission %q is duplicated", check.Name)
		}
		checks[check.Name] = check.Granted
	}
	missing := make([]string, 0)
	for _, permission := range request.RequiredPermissions {
		if !checks[permission] {
			missing = append(missing, permission)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("provider identity is missing required permissions: %s", strings.Join(missing, ", "))
	}
	return nil
}
