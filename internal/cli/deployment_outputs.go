package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/spf13/cobra"
)

type deploymentEndpoints struct {
	MagentoURL    string `json:"magentoUrl,omitempty" yaml:"magentoUrl,omitempty"`
	StorefrontURL string `json:"storefrontUrl,omitempty" yaml:"storefrontUrl,omitempty"`
	GraphQLURL    string `json:"graphqlUrl,omitempty" yaml:"graphqlUrl,omitempty"`
}

func deploymentLinks(application config.Application, providerURL string) *deploymentEndpoints {
	var magentoURL string
	for _, candidate := range []string{application.Magento.SecureBaseURL, application.Magento.UnsecureBaseURL, providerURL} {
		if magentoURL = deployHTTPURL(candidate); magentoURL != "" {
			break
		}
	}
	storefrontURL := magentoURL
	if application.Mode == "headless" {
		storefrontURL = deployHTTPURL(application.Magento.StorefrontOrigin)
	}
	if magentoURL == "" && storefrontURL == "" {
		return nil
	}
	var graphQLURL string
	if magentoURL != "" {
		graphQLURL, _ = url.JoinPath(magentoURL, "graphql")
	}
	return &deploymentEndpoints{MagentoURL: magentoURL, StorefrontURL: storefrontURL, GraphQLURL: graphQLURL}
}

func deployHTTPURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}

func (o *options) deploymentLogsCommand(cmd *cobra.Command, environment string) string {
	command := "magelift logs"
	if o.resolvedPreviewIdentity != nil {
		// Reuse the preview template and identity inputs passed to deploy.
		environment = o.environment
		if environment == "" {
			environment = o.getenv("MAGELIFT_ENV")
		}
	}
	if environment != "" {
		command += " --env " + environment
	}
	for _, name := range []string{
		"config", "preview-repository", "preview-number", "preview-branch", "preview-commit",
		"preview-domain", "preview-updated-at", "preview-expires-at", "preview-generation",
	} {
		if flag := cmd.Flag(name); flag != nil && flag.Changed {
			command += " --" + name + " " + shellQuote(flag.Value.String())
		}
	}
	return command + " --service web"
}

func validateDeploymentOutputPath(path string, reserved ...string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve deployment outputs path: %w", err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || !info.IsDir() {
		return errors.New("the deployment outputs directory must already exist")
	}
	destination, err := os.Lstat(path)
	if err == nil && !destination.Mode().IsRegular() {
		return errors.New("the deployment outputs path must be a regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect deployment outputs path: %w", err)
	}
	for _, input := range reserved {
		if input == "" {
			continue
		}
		if inputPath, err := filepath.Abs(input); err == nil && strings.EqualFold(inputPath, path) {
			return errors.New("choose a deployment outputs path separate from the project configuration and build manifest")
		}
		if source, err := os.Stat(input); err == nil && destination != nil && os.SameFile(source, destination) {
			return errors.New("choose a deployment outputs path separate from the project configuration and build manifest")
		}
	}
	return nil
}

func writeDeploymentOutputs(path string, result infrastructureResult) error {
	if result.Endpoints == nil || result.Endpoints.MagentoURL == "" {
		return errors.New("deploy succeeded, but MageLift could not read a Magento URL for the outputs file. Check magelift outputs")
	}
	report := struct {
		Environment string `json:"environment"`
		deploymentEndpoints
	}{Environment: result.Environment, deploymentEndpoints: *result.Endpoints}
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		err = writeGeneratedFile(path, append(data, '\n'), 0o600)
	}
	if err != nil {
		return fmt.Errorf("deploy succeeded, but writing the outputs file failed: %w", err)
	}
	return nil
}
