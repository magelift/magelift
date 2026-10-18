package githubci

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxRepoSegmentLen   = 100
	maxVariableValueLen = 48 << 10 // GitHub Actions variable limit
)

var allowedVariableNames = map[string]struct{}{
	"MAGELIFT_GCP_PROJECT_ID":                 {},
	"MAGELIFT_GCP_REGION":                     {},
	"MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER": {},
	"MAGELIFT_GCP_BUILD_SERVICE_ACCOUNT":      {},
	"MAGELIFT_GCP_TEST_SERVICE_ACCOUNT":       {},
	"MAGELIFT_GCP_STAGING_SERVICE_ACCOUNT":    {},
	"MAGELIFT_GCP_PRODUCTION_SERVICE_ACCOUNT": {},
	"MAGELIFT_GCP_PREVIEW_SERVICE_ACCOUNT":    {},
	"MAGELIFT_REGISTRY":                       {},
	"MAGELIFT_RELEASE_IMAGE":                  {},
	"MAGELIFT_IMAGE_SIGNING_IDENTITY":         {},
	"MAGELIFT_IMAGE_SIGNING_ISSUER":           {},
	"MAGELIFT_VERSION":                        {},
}

var sensitiveNameSubstrings = []string{
	"sensitive", "password", "token", "secret",
}

var credentialValuePrefixes = []string{
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_",
}

// ValidateVariable checks the nonsecret CI reference contract before any writes.
func ValidateVariable(name, value string) error {
	if err := validateVariableName(name); err != nil {
		return err
	}
	return validateVariableValue(value)
}

func validateRepoSegment(segment, label string) error {
	if segment == "" || segment == "." || segment == ".." {
		return fmt.Errorf("%s is required", label)
	}
	if len(segment) > maxRepoSegmentLen {
		return fmt.Errorf("%s exceeds maximum length", label)
	}
	for i := 0; i < len(segment); i++ {
		b := segment[i]
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.') {
			return fmt.Errorf("%s contains invalid characters", label)
		}
	}
	return nil
}

func validateVariableName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("variable name is required")
	}
	if _, ok := allowedVariableNames[name]; !ok {
		return fmt.Errorf("variable name %q is not allowed", name)
	}
	lower := strings.ToLower(name)
	for _, sub := range sensitiveNameSubstrings {
		if strings.Contains(lower, sub) {
			return fmt.Errorf("variable name %q is not allowed", name)
		}
	}
	return nil
}

func validateVariableValue(value string) error {
	if value == "" {
		return errors.New("variable value is required")
	}
	if len(value) > maxVariableValueLen {
		return errors.New("variable value exceeds maximum length")
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return errors.New("variable value must not contain control characters")
		}
	}
	trimmed := strings.TrimSpace(value)
	for _, prefix := range credentialValuePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return errors.New("variable value looks like a credential")
		}
	}
	return nil
}
