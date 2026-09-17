package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/spf13/cobra"
)

func providersCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "providers",
		Short: "Download and verify provider plugins",
	}
	command.AddCommand(providersInstallCommand(o))
	return command
}

func providersInstallCommand(o *options) *cobra.Command {
	var lockPath, cacheDir string
	command := &cobra.Command{
		Use:   "install",
		Short: "Download and verify the providers named by magelift.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := o.load()
			if err != nil {
				return err
			}
			providers, err := configProviders(file)
			if err != nil {
				return err
			}
			lock, lockDir, err := loadProviderLock(lockPath, o)
			if err != nil {
				return err
			}
			downloader := &providerhost.Downloader{
				Verifier: providerhost.NewCosignVerifier(),
				CacheDir: strings.TrimSpace(cacheDir),
				LockDir:  lockDir,
			}
			type result struct {
				Provider string `json:"provider"`
				Version  string `json:"version"`
				Binary   string `json:"binary"`
				Cached   bool   `json:"cached"`
				Bundled  bool   `json:"bundled"`
			}
			results := make([]result, 0, len(providers))
			for _, provider := range providers {
				artifact, err := lock.Artifact(provider)
				if err != nil {
					return &exitError{code: 3, err: err}
				}
				if strings.TrimSpace(artifact.URL) == "" {
					binary, ok := bundledProviderBinary(o, provider)
					if !ok {
						return &exitError{code: 3, err: fmt.Errorf("provider %q ships beside the CLI but no binary was found there; reinstall the CLI release", provider)}
					}
					results = append(results, result{Provider: provider, Version: strings.TrimSpace(artifact.Version), Binary: binary, Bundled: true})
					continue
				}
				installed, err := downloader.Install(cmd.Context(), provider, artifact)
				if err != nil {
					return &exitError{code: 3, err: err}
				}
				results = append(results, result{Provider: installed.Provider, Version: installed.Version, Binary: installed.Binary, Cached: installed.Cached})
			}
			return o.write(map[string]any{"providers": results})
		},
	}
	command.Flags().StringVar(&lockPath, "lockfile", "", "provider lockfile path (default: ./magelift.providers.lock, then beside the CLI)")
	command.Flags().StringVar(&cacheDir, "cache-dir", "", "provider cache directory (default: user cache)")
	return command
}

// configProviders returns the distinct providers named across every
// environment in the project file. Downloads are YAML-driven: no flag
// selects providers.
func configProviders(file *config.File) ([]string, error) {
	if file == nil {
		return nil, errors.New("project configuration is required")
	}
	environments := file.Environments()
	if len(environments) == 0 {
		return nil, errors.New("at least one environment is required")
	}
	seen := map[string]bool{}
	var providers []string
	for _, environment := range environments {
		effective, err := file.Resolve(environment, config.ResolveOptions{})
		if err != nil {
			return nil, fmt.Errorf("environment %s: %w", environment, err)
		}
		provider := strings.TrimSpace(effective.Config.Target.Provider)
		if provider == "" {
			return nil, fmt.Errorf("environment %s names no target provider", environment)
		}
		if !seen[provider] {
			seen[provider] = true
			providers = append(providers, provider)
		}
	}
	sort.Strings(providers)
	return providers, nil
}

func loadProviderLock(flag string, o *options) (providerhost.Lockfile, string, error) {
	candidates := []string{}
	if trimmed := strings.TrimSpace(flag); trimmed != "" {
		candidates = append(candidates, trimmed)
	} else {
		candidates = append(candidates, "magelift.providers.lock")
		executable := ""
		if o.executable != nil {
			if path, err := o.executable(); err == nil {
				executable = path
			}
		}
		dir := "."
		if executable != "" {
			dir = filepath.Dir(executable)
		}
		candidates = append(candidates, filepath.Join(dir, "magelift.providers.lock"))
	}
	var lastErr error
	for _, candidate := range candidates {
		file, err := os.Open(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		lock, parseErr := providerhost.ParseLock(file)
		file.Close()
		if parseErr != nil {
			return providerhost.Lockfile{}, "", parseErr
		}
		return lock, filepath.Dir(candidate), nil
	}
	if strings.TrimSpace(flag) != "" {
		return providerhost.Lockfile{}, "", fmt.Errorf("open %s: %w", strings.TrimSpace(flag), lastErr)
	}
	return providerhost.Lockfile{}, "", fmt.Errorf("no magelift.providers.lock in the project or beside the CLI: %w", lastErr)
}

func bundledProviderBinary(o *options, provider string) (string, bool) {
	executable := ""
	if o.executable != nil {
		if path, err := o.executable(); err == nil {
			executable = path
		}
	}
	binary := providerhost.DiscoverArtifactPaths(executable, provider).Binary
	info, err := os.Stat(binary)
	if err != nil || info.IsDir() {
		return "", false
	}
	return binary, true
}
