package cli

import (
	"context"
	"encoding/json"
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
	var lockPath, cacheDir, version string
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
			executable := ""
			if o.executable != nil {
				if path, err := o.executable(); err == nil {
					executable = path
				}
			}
			opts := providerhost.ResolveOptions{ExecutablePath: executable, LockPath: strings.TrimSpace(lockPath), CacheDir: strings.TrimSpace(cacheDir)}
			lockFile, lock, _, err := providerhost.ResolveLock(opts)
			bootstrapped := false
			if err != nil {
				if strings.TrimSpace(lockPath) != "" {
					return err
				}
				want := strings.TrimSpace(version)
				if want == "" {
					want = strings.TrimSpace(Version)
				}
				if !providerhost.ValidReleaseTag(want) {
					return &exitError{code: 3, err: fmt.Errorf("no provider lockfile and no release version to bootstrap from (running %q): pass --version vX.Y.Z", Version)}
				}
				fetch := o.fetchProviderLock
				if fetch == nil {
					fetch = func(ctx context.Context, v string) (providerhost.Lockfile, error) {
						return providerhost.FetchLock(ctx, v, "", nil)
					}
				}
				fetched, ferr := fetch(cmd.Context(), want)
				if ferr != nil {
					return &exitError{code: 3, err: ferr}
				}
				encoded, ferr := json.MarshalIndent(fetched, "", "  ")
				if ferr != nil {
					return &exitError{code: 3, err: ferr}
				}
				if ferr := os.WriteFile("magelift.providers.lock", append(encoded, '\n'), 0o644); ferr != nil {
					return &exitError{code: 3, err: fmt.Errorf("write bootstrapped lockfile: %w", ferr)}
				}
				lockFile, lock, bootstrapped = "magelift.providers.lock", fetched, true
			} else if strings.TrimSpace(version) != "" {
				return &exitError{code: 3, err: fmt.Errorf("provider lockfile %s already exists; remove it to re-bootstrap at --version %s", lockFile, strings.TrimSpace(version))}
			}
			cacheResolved, err := providerhost.EffectiveCacheDir(strings.TrimSpace(cacheDir))
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			ensure := o.ensureProviderVerifier
			if ensure == nil {
				ensure = func(ctx context.Context, dir string) (string, error) {
					return (&providerhost.VerifierInstaller{CacheDir: dir}).Ensure(ctx)
				}
			}
			verifierPath, err := ensure(cmd.Context(), cacheResolved)
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			downloader := &providerhost.Downloader{
				Verifier: providerhost.NewPreferredVerifier(cacheResolved),
				CacheDir: cacheResolved,
				LockDir:  filepath.Dir(lockFile),
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
			return o.write(map[string]any{"providers": results, "bootstrapped": bootstrapped, "verifier": verifierPath})
		},
	}
	command.Flags().StringVar(&lockPath, "lockfile", "", "provider lockfile path (default: ./magelift.providers.lock, then beside the CLI)")
	command.Flags().StringVar(&cacheDir, "cache-dir", "", "provider cache directory (default: user cache)")
	command.Flags().StringVar(&version, "version", "", "release tag to bootstrap the lockfile from when none exists (default: the CLI version)")
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
