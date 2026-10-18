package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/githubci"
	"github.com/magelift/magelift/internal/platform"
)

const (
	bootstrapDetailCIPermissionsApplied = "ciPermissionsApplied"
	bootstrapDetailCIVariables          = "ciVariables"
	bootstrapDetailGitHubRepositoryID   = "githubRepositoryId"
	bootstrapDetailGitHubOwnerID        = "githubOwnerId"
	bootstrapDetailGitHubRepoVariables  = "githubRepositoryVariables"
)

type bootstrapCIFlags struct {
	applyCIPermissions bool
	configureGitHub    bool
	githubOwner        string
	githubRepo         string
}

type bootstrapGitHubClient interface {
	ResolveRepository(ctx context.Context, owner, repo string) (githubci.Repository, error)
	EnsureVariable(ctx context.Context, owner, repo, name, value string) (githubci.EnsureVariableResult, error)
}

func validateBootstrapCIInputFlags(flags bootstrapCIFlags) error {
	_, _, wanted, err := (platform.BootstrapRequest{GitHubOwner: flags.githubOwner, GitHubRepo: flags.githubRepo}).GitHubIdentity()
	if err != nil {
		return err
	}
	if flags.configureGitHub && !flags.applyCIPermissions {
		return errors.New("--configure-github requires --apply-ci-permissions")
	}
	if flags.applyCIPermissions && !wanted {
		return errors.New("CI permissions require both --github-owner and --github-repo")
	}
	return nil
}

func validateBootstrapCIFlags(planned platform.PlannedStack, flags bootstrapCIFlags) error {
	if !flags.applyCIPermissions && !flags.configureGitHub {
		return nil
	}
	if planned == nil || planned.Provider() != "gcp" {
		return errors.New("--apply-ci-permissions and --configure-github are only supported for GCP targets")
	}

	return nil
}

func requireCIPermissionsApplied(details map[string]any) error {
	if details == nil {
		return errors.New("bootstrap did not confirm CI permissions were applied")
	}
	raw, ok := details[bootstrapDetailCIPermissionsApplied]
	if !ok {
		return errors.New("bootstrap did not confirm CI permissions were applied")
	}
	applied, ok := raw.(bool)
	if !ok {
		return errors.New("bootstrap returned an invalid CI permissions confirmation")
	}
	if !applied {
		return errors.New("bootstrap did not apply CI permissions")
	}
	return nil
}

func configureBootstrapGitHub(ctx context.Context, client bootstrapGitHubClient, owner, repo string, details map[string]any) ([]string, error) {
	if client == nil {
		return nil, errors.New("github client is not configured")
	}
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return nil, errors.New("github owner and repository are required")
	}
	wantRepoID, wantOwnerID, variables, err := parseBootstrapCIVariablesDetails(details)
	if err != nil {
		return nil, err
	}
	resolved, err := client.ResolveRepository(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	if decimalID(resolved.ID) != wantRepoID || decimalID(resolved.OwnerID) != wantOwnerID {
		return nil, errors.New("github repository identity does not match bootstrap metadata")
	}
	if !strings.EqualFold(resolved.OwnerLogin, owner) || !strings.EqualFold(resolved.Name, repo) {
		return nil, errors.New("GitHub canonical repository names do not match bootstrap metadata")
	}
	names := sortedBootstrapVariableNames(variables)
	var configured []string
	for _, name := range names {
		result, err := client.EnsureVariable(ctx, resolved.OwnerLogin, resolved.Name, name, variables[name])
		if err != nil {
			return nil, err
		}
		if result.Name != name {
			return nil, fmt.Errorf("github repository variable %q was not configured", name)
		}
		configured = append(configured, name)
	}
	return configured, nil
}

func parseBootstrapCIVariablesDetails(details map[string]any) (repoID, ownerID string, variables map[string]string, err error) {
	if details == nil {
		return "", "", nil, errors.New("bootstrap did not return github repository metadata")
	}
	repoID, err = positiveDecimalDetail(details, bootstrapDetailGitHubRepositoryID)
	if err != nil {
		return "", "", nil, err
	}
	ownerID, err = positiveDecimalDetail(details, bootstrapDetailGitHubOwnerID)
	if err != nil {
		return "", "", nil, err
	}
	variables, err = stringMapDetail(details, bootstrapDetailCIVariables)
	if err != nil {
		return "", "", nil, err
	}
	if len(variables) == 0 {
		return "", "", nil, errors.New("bootstrap did not return CI variables for github configuration")
	}
	if err := validateBootstrapCIVariableMap(variables); err != nil {
		return "", "", nil, err
	}
	return repoID, ownerID, variables, nil
}

func validateBootstrapCIVariableMap(variables map[string]string) error {
	for name, value := range variables {
		if err := githubci.ValidateVariable(name, value); err != nil {
			return err
		}
	}
	return nil
}

func sortedBootstrapVariableNames(variables map[string]string) []string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func positiveDecimalDetail(details map[string]any, key string) (string, error) {
	raw, ok := details[key]
	if !ok {
		return "", fmt.Errorf("bootstrap did not return %s", key)
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("bootstrap returned invalid %s", key)
	}
	text = strings.TrimSpace(text)
	if text == "" || text == "0" {
		return "", fmt.Errorf("bootstrap returned invalid %s", key)
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return "", fmt.Errorf("bootstrap returned invalid %s", key)
		}
	}
	return text, nil
}

func stringMapDetail(details map[string]any, key string) (map[string]string, error) {
	raw, ok := details[key]
	if !ok {
		return nil, fmt.Errorf("bootstrap did not return %s", key)
	}
	switch typed := raw.(type) {
	case map[string]string:
		return typed, nil
	case map[string]any:
		out := make(map[string]string, len(typed))
		for name, value := range typed {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("bootstrap CI variable %q is not a string", name)
			}
			out[name] = text
		}
		return out, nil
	default:
		return nil, fmt.Errorf("bootstrap returned invalid %s", key)
	}
}

func decimalID(id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", id)
}

func bootstrapNextCommand(o *options, planned platform.PlannedStack, flags bootstrapCIFlags, githubConfigured bool) string {
	env := "preview"
	if planned != nil && planned.Environment() != "" {
		env = planned.Environment()
	}
	if githubConfigured {
		return joinMageliftCommand(o, "ci", "generate")
	}
	if flags.githubOwner != "" && flags.githubRepo != "" && !flags.configureGitHub && planned != nil && planned.Provider() == "gcp" {
		return bootstrapGitHubReviewCommand(o, env, flags.githubOwner, flags.githubRepo)
	}
	return "magelift deploy --env " + env + " --yes"
}

func bootstrapGitHubReviewCommand(o *options, env, owner, repo string) string {
	parts := []string{"magelift"}
	if prefix := bootstrapConfigArgs(o); prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts,
		"--env", shellQuote(env),
		"bootstrap",
		"--github-owner", shellQuote(owner),
		"--github-repo", shellQuote(repo),
		"--apply-ci-permissions",
		"--configure-github",
	)
	return strings.Join(parts, " ")
}

func joinMageliftCommand(o *options, subcommand string, args ...string) string {
	parts := []string{"magelift"}
	if prefix := bootstrapConfigArgs(o); prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, subcommand)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func bootstrapConfigArgs(o *options) string {
	if o == nil {
		return ""
	}
	path := strings.TrimSpace(o.configPath)
	if path == "" || path == "magelift.yaml" {
		return ""
	}
	return "--config " + shellQuote(path)
}

func (o *options) bootstrapGitHubClient(ctx context.Context) (bootstrapGitHubClient, error) {
	if o.newBootstrapGitHub != nil {
		return o.newBootstrapGitHub(ctx)
	}
	return githubci.NewFromEnvironment(ctx)
}
