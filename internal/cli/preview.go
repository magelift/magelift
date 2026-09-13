package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func (o *options) resolveEnvironment(file *config.File, environment string) (config.Effective, string, error) {
	effective, err := file.Resolve(environment, config.ResolveOptions{})
	if err != nil {
		return config.Effective{}, "", err
	}
	identity, err := o.previewIdentity(effective.Config)
	if err != nil {
		return config.Effective{}, "", err
	}
	if o.previewIdentityOverride != nil {
		override := *o.previewIdentityOverride
		if err := override.Validate(); err != nil {
			return config.Effective{}, "", fmt.Errorf("validate preview identity override: %w", err)
		}
		if override.Project != effective.Config.Project.Name || effective.Config.Class != "preview" {
			return config.Effective{}, "", errors.New("preview identity override must target the selected preview environment")
		}
		identity = &override
	}
	if identity == nil {
		return effective, environment, nil
	}
	effective, err = file.Resolve(environment, config.ResolveOptions{PreviewIdentity: identity})
	if err != nil {
		return config.Effective{}, "", err
	}
	return effective, identity.Environment, nil
}

func (o *options) previewIdentity(effective config.Config) (*config.PreviewIdentity, error) {
	if !o.previewIdentityRequested() {
		return nil, nil
	}
	if effective.Class != "preview" {
		return nil, errors.New("preview identity inputs require an environment with class preview")
	}
	repository := strings.TrimSpace(o.previewRepository)
	if repository == "" {
		return nil, errors.New("--preview-repository is required when preview identity inputs are set")
	}
	if o.previewPullRequest <= 0 {
		return nil, errors.New("--preview-number must be greater than zero")
	}
	domain := effective.Domain
	if strings.TrimSpace(o.previewDomain) != "" {
		domain = o.previewDomain
	} else if baseDomain := strings.TrimSuffix(strings.TrimSpace(effective.Domain), "."); baseDomain != "" {
		domain = "pr-" + strconv.FormatInt(o.previewPullRequest, 10) + "." + baseDomain
	}
	identity, err := config.BuildPreviewIdentity(config.PreviewIdentityInput{
		Project:     effective.Project.Name,
		Repository:  repository,
		PullRequest: o.previewPullRequest,
		Branch:      o.previewBranch,
		Commit:      o.previewCommit,
		Generation:  o.previewGeneration,
		Domain:      domain,
		ExpiresAt:   effective.ExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("build preview identity: %w", err)
	}
	return &identity, nil
}

func (o *options) previewIdentityRequested() bool {
	return strings.TrimSpace(o.previewRepository) != "" ||
		o.previewPullRequest != 0 ||
		strings.TrimSpace(o.previewBranch) != "" ||
		strings.TrimSpace(o.previewCommit) != "" ||
		strings.TrimSpace(o.previewDomain) != "" ||
		o.previewGeneration != 0
}

func previewMetadata(identity *config.PreviewIdentity) *automation.PreviewMetadata {
	if identity == nil {
		return nil
	}
	return &automation.PreviewMetadata{
		Project:      identity.Project,
		Repository:   identity.Repository,
		PullRequest:  identity.PullRequest,
		Environment:  identity.Environment,
		StackKey:     identity.StackKey,
		Owner:        identity.OwnershipMarker,
		Branch:       identity.Branch,
		CommitDigest: identity.CommitDigest,
		Domain:       identity.Domain,
		ExpiresAt:    identity.ExpiresAt,
		Generation:   identity.Generation,
	}
}

func (o *options) automationRequest(target sdk.TargetDescriptor, destroying bool) automation.Request {
	return automation.Request{
		Target:  target,
		Preview: previewMetadata(o.resolvedPreviewIdentity),
		Destroy: destroying,
	}
}
