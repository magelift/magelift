package secretsafe

import (
	"encoding/json"
	"errors"
	"regexp"
)

var secretSafetyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:password|secret|token|api[_-]?key|api[_-]?token|access[_-]?key|private[_-]?key|client[_-]?secret|client[_-]?certificate|certificate[_-]?(?:key|pem)|dns[_-]?(?:key|token)|recovery[_-]?(?:key|token))["']?\s*[:=]\s*["']?[A-Za-z0-9_./+=:-]{8,}`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{20,}`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`-----BEGIN(?: [A-Z]+)? PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|redis|rediss|amqp|amqps)://[^\s/@:]+:[^\s/@]+@`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
}

var ErrSecretMaterial = errors.New("secret-like assignment")

// ValidateSecretSafeText rejects plaintext credential material without
// returning the matched value. Opaque secret references, resource IDs, and
// ordinary certificate identities remain valid because they do not match a
// credential shape.
func ValidateSecretSafeText(value string) error {
	for _, pattern := range secretSafetyPatterns {
		if pattern.MatchString(value) {
			return ErrSecretMaterial
		}
	}
	return nil
}

// ValidateSecretSafeValue applies the same check to structured plans,
// checkpoints, evidence, and adapter output. JSON is used only as a
// deterministic traversal representation; it is never written by this
// function.
func ValidateSecretSafeValue(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return ValidateSecretSafeText(string(data))
}

// RedactSensitiveText returns a safe diagnostic form and whether any
// credential-shaped material was replaced. Callers should record only the
// returned text when emitting untrusted provider output.
func RedactSensitiveText(value string) (string, bool) {
	redacted := value
	for _, pattern := range secretSafetyPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted, redacted != value
}
