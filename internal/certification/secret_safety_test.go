package certification

import (
	"strings"
	"testing"
)

func TestSecretSafetyRejectsProviderAndRecoveryCredentials(t *testing.T) {
	values := []string{
		`{"awsAccessKey":"AKIA1234567890ABCDEF"}`,
		`Authorization: Bearer eyJaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbb.cccccccccccc`,
		`postgres://user:password@example.invalid/database`,
		`{"fastlyApiToken":"abcdefghijklmnop"}`,
		`-----BEGIN PRIVATE KEY-----`,
		`{"recoveryToken":"abcdefghijklmnop"}`,
	}
	for _, value := range values {
		if err := ValidateSecretSafeText(value); err == nil {
			t.Errorf("plaintext credential was accepted: %q", value)
		}
	}
}

func TestSecretSafetyAllowsOpaqueReferencesAndRedactsDiagnostics(t *testing.T) {
	for _, value := range []string{
		`aws-secrets-manager://magelift/certification/database`,
		`certificate-identity-release-1`,
		`arn:aws:iam::123456789012:role/magelift-certifier`,
	} {
		if err := ValidateSecretSafeText(value); err != nil {
			t.Errorf("opaque reference was rejected: %q: %v", value, err)
		}
	}
	input := `provider=aws token=abcdefghijklmnop`
	redacted, changed := RedactSensitiveText(input)
	if !changed || strings.Contains(redacted, "abcdefghijklmnop") || !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("redaction = %q changed=%v", redacted, changed)
	}
}
