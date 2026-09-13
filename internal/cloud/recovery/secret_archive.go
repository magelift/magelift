package recovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SecretArchive is the provider-neutral encrypted-archive envelope used by
// secret-manager translators. Providers own the storage encryption policy;
// this type only makes the payload and scope deterministic and verifiable.
// Value is never returned as lifecycle evidence or included in an operation
// identity.
type SecretArchive struct {
	Version         int    `json:"version"`
	DataClass       string `json:"dataClass"`
	FixtureID       string `json:"fixtureId"`
	OwnershipMarker string `json:"ownershipMarker"`
	SourceSecret    string `json:"sourceSecret"`
	Encoding        string `json:"encoding"`
	Value           []byte `json:"value"`
	Digest          string `json:"digest"`
}

// SealSecretArchive serializes and seals an archive without exposing its
// contents to any caller other than the provider storage boundary.
func SealSecretArchive(archive SecretArchive) ([]byte, string, error) {
	archive.Digest = ""
	if err := validateSecretArchive(archive); err != nil {
		return nil, "", err
	}
	unsigned, err := json.Marshal(archive)
	if err != nil {
		return nil, "", err
	}
	archive.Digest = Digest(unsigned)
	sealed, err := json.Marshal(archive)
	if err != nil {
		return nil, "", err
	}
	return sealed, archive.Digest, nil
}

// OpenSecretArchive verifies and decodes a sealed archive.
func OpenSecretArchive(body []byte) (SecretArchive, string, error) {
	var archive SecretArchive
	if err := json.Unmarshal(body, &archive); err != nil {
		return SecretArchive{}, "", err
	}
	if strings.TrimSpace(archive.Digest) == "" {
		return SecretArchive{}, "", errors.New("secret archive is unsigned")
	}
	expected := archive.Digest
	archive.Digest = ""
	if err := validateSecretArchive(archive); err != nil {
		return SecretArchive{}, "", err
	}
	unsigned, err := json.Marshal(archive)
	if err != nil {
		return SecretArchive{}, "", err
	}
	if Digest(unsigned) != expected {
		return SecretArchive{}, "", errors.New("secret archive digest mismatch")
	}
	archive.Digest = expected
	return archive, expected, nil
}

// ValidateSecretArchive verifies that an archive belongs to the requested
// provider-neutral recovery scope.
func ValidateSecretArchive(archive SecretArchive, version int, dataClass, fixtureID, ownershipMarker string) error {
	if archive.Version != version || archive.DataClass != dataClass || archive.FixtureID != fixtureID || archive.OwnershipMarker != ownershipMarker {
		return errors.New("secret archive does not match the operation scope")
	}
	return validateSecretArchive(archive)
}

func validateSecretArchive(archive SecretArchive) error {
	if archive.Version <= 0 || strings.TrimSpace(archive.DataClass) == "" || strings.TrimSpace(archive.FixtureID) == "" || strings.TrimSpace(archive.OwnershipMarker) == "" || strings.TrimSpace(archive.SourceSecret) == "" {
		return errors.New("secret archive scope is incomplete")
	}
	if archive.Encoding != "string" && archive.Encoding != "binary" {
		return fmt.Errorf("secret archive encoding %q is unsupported", archive.Encoding)
	}
	if archive.Value == nil {
		return errors.New("secret archive value is missing")
	}
	return nil
}
