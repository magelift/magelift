package config

import (
	"regexp"
	"strings"
)

// CI environment references apply only to build-time Composer credentials.
// Runtime secrets still use the provider's existing reference contract.
func validateComposerCredentialsReference(reference, provider string) []string {
	if strings.HasPrefix(reference, "env://") {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(strings.TrimPrefix(reference, "env://")) {
			return []string{"build.composer.credentials env reference must name one environment variable"}
		}
		return nil
	}
	return validateProviderSecretReference("build.composer.credentials", reference, provider)
}
