package certification

import "github.com/magelift/magelift/internal/secretsafe"

// ErrSecretMaterial reports credential-shaped plaintext. The vocabulary
// lives in internal/secretsafe so leaf packages can share the single
// definition without importing certification; these aliases keep
// existing certification callers unchanged.
var ErrSecretMaterial = secretsafe.ErrSecretMaterial

// ValidateSecretSafeText rejects plaintext credential material.
func ValidateSecretSafeText(value string) error {
	return secretsafe.ValidateSecretSafeText(value)
}

// ValidateSecretSafeValue applies the credential check to structured values.
func ValidateSecretSafeValue(value any) error {
	return secretsafe.ValidateSecretSafeValue(value)
}

// RedactSensitiveText returns a safe diagnostic form and whether anything
// was replaced.
func RedactSensitiveText(value string) (string, bool) {
	return secretsafe.RedactSensitiveText(value)
}
