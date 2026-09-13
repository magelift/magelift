package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	previewProjectPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	previewRepositoryPart     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	previewEnvironmentPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	previewCommitPattern      = regexp.MustCompile(`^[a-f0-9]{7,64}$`)
)

// PreviewIdentityInput contains the CI metadata used to derive one stable
// pull-request preview. Branch and commit are diagnostic metadata; the
// repository and pull request number define the identity.
type PreviewIdentityInput struct {
	Project     string
	Repository  string
	PullRequest int64
	Branch      string
	Commit      string
	Generation  uint64
	Domain      string
	ExpiresAt   string
}

// PreviewIdentity is the provider-neutral identity carried by a resolved
// preview configuration. It contains no credentials or provider SDK values.
type PreviewIdentity struct {
	Project          string `yaml:"project" json:"project"`
	Repository       string `yaml:"repository" json:"repository"`
	RepositoryDigest string `yaml:"repositoryDigest" json:"repositoryDigest"`
	PullRequest      int64  `yaml:"pullRequest" json:"pullRequest"`
	Environment      string `yaml:"environment" json:"environment"`
	StackKey         string `yaml:"stackKey" json:"stackKey"`
	OwnershipMarker  string `yaml:"ownershipMarker" json:"ownershipMarker"`
	Domain           string `yaml:"domain,omitempty" json:"domain,omitempty"`
	Branch           string `yaml:"branch,omitempty" json:"branch,omitempty"`
	CommitDigest     string `yaml:"commitDigest,omitempty" json:"commitDigest,omitempty"`
	Generation       uint64 `yaml:"generation" json:"generation"`
	ExpiresAt        string `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty"`
}

// BuildPreviewIdentity derives deterministic names and ownership values from
// repository and pull-request metadata. A zero generation means the first
// deployment and is normalized to generation one.
func BuildPreviewIdentity(input PreviewIdentityInput) (PreviewIdentity, error) {
	project := strings.TrimSpace(input.Project)
	if !previewProjectPattern.MatchString(project) {
		return PreviewIdentity{}, errors.New("preview project must be a stable lowercase name")
	}
	repository, err := canonicalRepository(input.Repository)
	if err != nil {
		return PreviewIdentity{}, err
	}
	if input.PullRequest <= 0 {
		return PreviewIdentity{}, errors.New("preview pull request must be greater than zero")
	}
	branch, err := previewMetadataValue("branch", input.Branch, 255)
	if err != nil {
		return PreviewIdentity{}, err
	}
	commit, err := canonicalCommit(input.Commit)
	if err != nil {
		return PreviewIdentity{}, err
	}
	domain, err := previewMetadataValue("domain", input.Domain, 253)
	if err != nil {
		return PreviewIdentity{}, err
	}
	expiresAt, err := canonicalPreviewTime(input.ExpiresAt)
	if err != nil {
		return PreviewIdentity{}, err
	}
	generation := input.Generation
	if generation == 0 {
		generation = 1
	}

	repositoryDigestBytes := sha256.Sum256([]byte(repository))
	repositoryDigest := hex.EncodeToString(repositoryDigestBytes[:])
	shortDigest := repositoryDigest[:12]
	pullRequest := strconv.FormatInt(input.PullRequest, 10)
	identity := PreviewIdentity{
		Project:          project,
		Repository:       repository,
		RepositoryDigest: repositoryDigest,
		PullRequest:      input.PullRequest,
		Environment:      "pr-" + pullRequest + "-" + shortDigest,
		StackKey:         "preview-" + shortDigest + "-pr-" + pullRequest,
		OwnershipMarker:  "magelift-preview-" + shortDigest + "-pr-" + pullRequest,
		Domain:           domain,
		Branch:           branch,
		CommitDigest:     commit,
		Generation:       generation,
		ExpiresAt:        expiresAt,
	}
	if err := identity.Validate(); err != nil {
		return PreviewIdentity{}, fmt.Errorf("validate derived preview identity: %w", err)
	}
	return identity, nil
}

// Validate verifies both metadata safety and the derived identity invariants.
func (identity PreviewIdentity) Validate() error {
	if !previewProjectPattern.MatchString(identity.Project) {
		return errors.New("preview project must be a stable lowercase name")
	}
	repository, err := canonicalRepository(identity.Repository)
	if err != nil {
		return err
	}
	if repository != identity.Repository {
		return errors.New("preview repository must be canonical lowercase owner/repository")
	}
	if identity.PullRequest <= 0 {
		return errors.New("preview pull request must be greater than zero")
	}
	if len(identity.RepositoryDigest) != sha256.Size*2 {
		return errors.New("preview repository digest must be a SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(identity.RepositoryDigest); err != nil {
		return errors.New("preview repository digest must be a SHA-256 hex digest")
	}
	expectedDigest := sha256.Sum256([]byte(identity.Repository))
	if identity.RepositoryDigest != hex.EncodeToString(expectedDigest[:]) {
		return errors.New("preview repository digest does not match repository")
	}
	pullRequest := strconv.FormatInt(identity.PullRequest, 10)
	shortDigest := identity.RepositoryDigest[:12]
	if identity.Environment != "pr-"+pullRequest+"-"+shortDigest || !previewEnvironmentPattern.MatchString(identity.Environment) {
		return errors.New("preview environment does not match repository and pull request identity")
	}
	if identity.StackKey != "preview-"+shortDigest+"-pr-"+pullRequest {
		return errors.New("preview stack key does not match repository and pull request identity")
	}
	if identity.OwnershipMarker != "magelift-preview-"+shortDigest+"-pr-"+pullRequest {
		return errors.New("preview ownership marker does not match repository and pull request identity")
	}
	if identity.Generation == 0 {
		return errors.New("preview generation must be greater than zero")
	}
	if _, err := previewMetadataValue("branch", identity.Branch, 255); err != nil {
		return err
	}
	if _, err := canonicalCommit(identity.CommitDigest); err != nil {
		return err
	}
	if _, err := previewMetadataValue("domain", identity.Domain, 253); err != nil {
		return err
	}
	canonicalExpiresAt, err := canonicalPreviewTime(identity.ExpiresAt)
	if err != nil {
		return err
	}
	if canonicalExpiresAt != identity.ExpiresAt {
		return errors.New("preview expiration must be canonical RFC3339")
	}
	return nil
}

func canonicalRepository(value string) (string, error) {
	repository := strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(repository, "https://github.com/"):
		repository = strings.TrimPrefix(repository, "https://github.com/")
	case strings.HasPrefix(repository, "http://github.com/"):
		repository = strings.TrimPrefix(repository, "http://github.com/")
	case strings.HasPrefix(repository, "git@github.com:"):
		repository = strings.TrimPrefix(repository, "git@github.com:")
	}
	repository = strings.TrimSuffix(repository, ".git")
	repository = strings.ToLower(repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || !previewRepositoryPart.MatchString(parts[0]) || !previewRepositoryPart.MatchString(parts[1]) {
		return "", errors.New("preview repository must be owner/repository or a GitHub repository URL")
	}
	return parts[0] + "/" + parts[1], nil
}

// CanonicalPreviewRepository normalizes a GitHub repository reference for
// preview ownership filters and external lifecycle integrations.
func CanonicalPreviewRepository(value string) (string, error) {
	return canonicalRepository(value)
}

func canonicalCommit(value string) (string, error) {
	commit := strings.ToLower(strings.TrimSpace(value))
	if commit == "" {
		return "", nil
	}
	if !previewCommitPattern.MatchString(commit) {
		return "", errors.New("preview commit digest must contain 7 to 64 lowercase hexadecimal characters")
	}
	return commit, nil
}

func previewMetadataValue(name, value string, maxLength int) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > maxLength {
		return "", fmt.Errorf("preview %s exceeds %d characters", name, maxLength)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("preview %s must not contain control characters", name)
	}
	return value, nil
}

func canonicalPreviewTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("preview expiration must be RFC3339: %w", err)
	}
	return parsed.UTC().Format(time.RFC3339), nil
}
