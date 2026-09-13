package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ImmutableArtifactContract is the release artifact identity shared by every
// compatible certification cell. It is deliberately independent of a
// registry, signer, or build system so community builders can provide the
// same evidence without importing a MageLift implementation package.
type ImmutableArtifactContract struct {
	ImageDigest         string `json:"imageDigest" yaml:"imageDigest"`
	ManifestDigest      string `json:"manifestDigest" yaml:"manifestDigest"`
	InputFingerprint    string `json:"inputFingerprint" yaml:"inputFingerprint"`
	ProvenanceReference string `json:"provenanceReference" yaml:"provenanceReference"`
	SignatureReference  string `json:"signatureReference" yaml:"signatureReference"`
}

var artifactOCIReferencePattern = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
var artifactDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var artifactFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Validate requires the exact immutable and provenance identities needed to
// reuse one build across compatible provider cells. Secrets are never part of
// this contract; references remain opaque and single-line.
func (a ImmutableArtifactContract) Validate() error {
	var problems []error
	if !artifactOCIReferencePattern.MatchString(a.ImageDigest) {
		problems = append(problems, errors.New("artifact image digest must be a registry-qualified OCI SHA-256 reference"))
	}
	if !artifactDigestPattern.MatchString(a.ManifestDigest) {
		problems = append(problems, errors.New("artifact manifest digest must be a SHA-256 digest"))
	}
	if !artifactFingerprintPattern.MatchString(a.InputFingerprint) {
		problems = append(problems, errors.New("artifact input fingerprint must be a SHA-256 digest"))
	}
	for name, value := range map[string]string{
		"provenance reference": a.ProvenanceReference,
		"signature reference":  a.SignatureReference,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("artifact %s is required and must be single-line", name))
		}
	}
	return errors.Join(problems...)
}

// ReuseKey is the stable identity used to prove that one immutable artifact
// was reused. It includes provenance and signature references so a retagged
// or rebuilt image cannot accidentally share a certification result.
func (a ImmutableArtifactContract) ReuseKey() (string, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("encode artifact contract: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
