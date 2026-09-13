package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const previewOwnershipTag = "magelift:preview:identity"

var (
	ErrPreviewOwnershipConflict = errors.New("preview ownership conflict")
	ErrPreviewOwnershipMissing  = errors.New("preview ownership metadata is missing")
	ErrPreviewUnownedStack      = errors.New("preview stack is already populated without MageLift ownership metadata")
	ErrPreviewMetadataInvalid   = errors.New("preview ownership metadata is invalid")
)

// PreviewMetadata is the provider-neutral lifecycle record carried with a
// preview request. It contains identifiers and deployment metadata only.
type PreviewMetadata struct {
	Project      string `json:"project" yaml:"project"`
	Repository   string `json:"repository" yaml:"repository"`
	PullRequest  int64  `json:"pullRequest" yaml:"pullRequest"`
	Environment  string `json:"environment" yaml:"environment"`
	StackKey     string `json:"stackKey" yaml:"stackKey"`
	Owner        string `json:"owner" yaml:"owner"`
	Branch       string `json:"branch,omitempty" yaml:"branch,omitempty"`
	CommitDigest string `json:"commitDigest,omitempty" yaml:"commitDigest,omitempty"`
	Domain       string `json:"domain,omitempty" yaml:"domain,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
	Generation   uint64 `json:"generation" yaml:"generation"`
}

func (m PreviewMetadata) Validate() error {
	for field, value := range map[string]string{
		"project":     m.Project,
		"repository":  m.Repository,
		"environment": m.Environment,
		"stack key":   m.StackKey,
		"owner":       m.Owner,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("preview %s is required", field)
		}
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return fmt.Errorf("preview %s contains a control character", field)
		}
	}
	if m.PullRequest <= 0 {
		return errors.New("preview pull request must be greater than zero")
	}
	if m.Generation == 0 {
		return errors.New("preview generation must be greater than zero")
	}
	for field, value := range map[string]string{
		"branch":        m.Branch,
		"commit digest": m.CommitDigest,
		"domain":        m.Domain,
		"expiration":    m.ExpiresAt,
	} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return fmt.Errorf("preview %s contains a control character", field)
		}
	}
	return nil
}

type PreviewRecord struct {
	StackName string          `json:"stackName" yaml:"stackName"`
	Metadata  PreviewMetadata `json:"metadata" yaml:"metadata"`
}

func encodePreviewMetadata(metadata PreviewMetadata) (string, error) {
	if err := metadata.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode preview ownership metadata: %w", err)
	}
	return string(payload), nil
}

func decodePreviewMetadata(value string) (PreviewMetadata, error) {
	var metadata PreviewMetadata
	if strings.TrimSpace(value) == "" {
		return metadata, ErrPreviewOwnershipMissing
	}
	if err := json.Unmarshal([]byte(value), &metadata); err != nil {
		return metadata, fmt.Errorf("%w: decode persisted record: %v", ErrPreviewMetadataInvalid, err)
	}
	if err := metadata.Validate(); err != nil {
		return metadata, fmt.Errorf("%w: %v", ErrPreviewMetadataInvalid, err)
	}
	return metadata, nil
}

type PreviewOwnershipError struct {
	Cause     error
	Operation string
	Reason    string
	Requested PreviewMetadata
	Current   *PreviewMetadata
}

func (e *PreviewOwnershipError) Error() string {
	if e == nil {
		return "preview ownership error"
	}
	message := "preview ownership check failed"
	if e.Operation != "" {
		message = fmt.Sprintf("preview %s ownership check failed", e.Operation)
	}
	if e.Reason != "" {
		message += ": " + e.Reason
	}
	if e.Current != nil {
		message += fmt.Sprintf("; current generation is %d", e.Current.Generation)
	}
	message += "; retry with the current preview generation after inspecting the preview record"
	return message
}

func (e *PreviewOwnershipError) Unwrap() error {
	if e == nil || e.Cause == nil {
		return ErrPreviewOwnershipConflict
	}
	return e.Cause
}

func ownershipError(operation string, cause error, reason string, requested PreviewMetadata, current *PreviewMetadata) error {
	var currentCopy *PreviewMetadata
	if current != nil {
		copy := *current
		currentCopy = &copy
	}
	return &PreviewOwnershipError{
		Cause:     cause,
		Operation: operation,
		Reason:    reason,
		Requested: requested,
		Current:   currentCopy,
	}
}
