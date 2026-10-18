package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/magelift/magelift/internal/secretref"
)

var sensitiveConfigurationKey = regexp.MustCompile(`(?i)(?:password|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)`)

// ResolvedFingerprint returns the identity of the fully resolved configuration
// after replacing raw sensitive values with a stable redaction marker. The
// value is suitable for architecture and reuse fingerprints; provider SDK
// types and secret values never enter the core contract.
func ResolvedFingerprint(cfg Config) (string, error) {
	redacted, err := normalizedSanitizedConfiguration(cfg)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(redacted)
	if err != nil {
		return "", fmt.Errorf("encode redacted configuration: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// SafeEffectiveOutput returns the effective configuration and provenance in a
// form safe for CLI output. Secret references remain inspectable, but a raw
// value under a sensitive key is replaced before it can reach JSON or YAML.
func (e Effective) SafeEffectiveOutput() map[string]any {
	return map[string]any{
		"config":        safeSanitizedConfiguration(e.Config),
		"provenance":    e.Provenance,
		"compatibility": e.Compatibility,
		"fingerprint":   e.Fingerprint,
	}
}

func normalizedSanitizedConfiguration(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode resolved configuration: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("decode resolved configuration: %w", err)
	}
	return sanitizeConfigurationValue(normalized), nil
}

func safeSanitizedConfiguration(value any) any {
	result, err := normalizedSanitizedConfiguration(value)
	if err != nil {
		return "<unavailable>"
	}
	return result
}

func sanitizeConfigurationValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, nested := range value {
			if sensitiveConfigurationKey.MatchString(key) {
				if reference, ok := nested.(string); ok && isSecretReference(reference) {
					result[key] = reference
				} else {
					result[key] = "<redacted>"
				}
				continue
			}
			result[key] = sanitizeConfigurationValue(nested)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, nested := range value {
			result[index] = sanitizeConfigurationValue(nested)
		}
		return result
	default:
		return value
	}
}

func isSecretReference(value string) bool {
	_, err := secretref.Parse(value)
	return err == nil
}
