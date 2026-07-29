package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	buildrunner "github.com/acourtiol/magelift/internal/build/runner"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/source"
)

var immutableInputs = []string{"composer.lock", "app/etc/config.php"}

func PrepareRequest(file *config.File, repository source.Repository, runnerRoot string) (buildrunner.Request, error) {
	spec, err := file.ResolveBuild()
	if err != nil {
		return buildrunner.Request{}, err
	}
	inputs, err := hashInputs(repository.Root)
	if err != nil {
		return buildrunner.Request{}, err
	}
	staticContent, err := staticContent(spec.Build.StaticContent)
	if err != nil {
		return buildrunner.Request{}, err
	}
	lifecycleHooks, err := lifecycleHooks(spec.Build.Hooks)
	if err != nil {
		return buildrunner.Request{}, err
	}
	request := buildrunner.Request{
		ProtocolVersion: buildrunner.ProtocolVersion,
		Stage:           buildrunner.StagePrepare,
		Prepare: &buildrunner.PrepareRequest{
			RepositoryRoot: runnerRoot,
			SourceRevision: repository.Revision,
			Application: buildrunner.Application{
				Edition:    spec.Application.Edition,
				Version:    spec.Application.Version,
				Mode:       spec.Application.Mode,
				WebRuntime: spec.Application.WebRuntime,
			},
			PHPVersion:          spec.Build.PHP,
			CompatibilityStatus: string(spec.Compatibility.Status),
			InputFiles:          inputs,
			StaticContent:       staticContent,
			LifecycleHooks:      lifecycleHooks,
		},
	}
	if err := request.Validate(); err != nil {
		return buildrunner.Request{}, fmt.Errorf("create build runner request: %w", err)
	}
	return request, nil
}

func lifecycleHooks(settings map[string]config.BuildHook) ([]buildrunner.LifecycleHook, error) {
	ids := make([]string, 0, len(settings))
	for id := range settings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]buildrunner.LifecycleHook, 0, len(ids))
	for _, id := range ids {
		hook := settings[id]
		converted := buildrunner.LifecycleHook{
			ID:           id,
			Phase:        hook.Phase,
			Relationship: hook.Relationship,
			Target:       hook.Target,
			Dependencies: append([]string(nil), hook.Dependencies...),
			Timeout:      hook.Timeout,
			Retries: buildrunner.HookRetries{
				MaxAttempts:  hook.Retries.MaxAttempts,
				DelaySeconds: hook.Retries.DelaySeconds,
				Idempotent:   hook.Retries.Idempotent,
			},
			Failure: hook.Failure,
		}
		if hook.Command != nil {
			converted.Command = &buildrunner.HookCommand{
				Executable: hook.Command.Executable,
				Arguments:  append([]string(nil), hook.Command.Arguments...),
			}
		}
		result = append(result, converted)
	}

	return result, nil
}

func hashInputs(root string) ([]buildrunner.InputFile, error) {
	inputs := make([]buildrunner.InputFile, 0, len(immutableInputs))
	for _, relative := range immutableInputs {
		path := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && relative != "composer.lock" {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect immutable build input %s: %w", relative, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("immutable build input %s must be a regular file", relative)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read immutable build input %s: %w", relative, err)
		}
		sum := sha256.Sum256(contents)
		inputs = append(inputs, buildrunner.InputFile{Path: relative, SHA256: hex.EncodeToString(sum[:])})
	}
	return inputs, nil
}

func staticContent(settings map[string]any) ([]buildrunner.StaticContent, error) {
	locales, err := stringList(settings, "locales")
	if err != nil {
		return nil, err
	}
	themes, err := stringList(settings, "themes")
	if err != nil {
		return nil, err
	}
	if (len(locales) == 0) != (len(themes) == 0) {
		return nil, errors.New("build.staticContent.locales and themes must be configured together")
	}
	strategy, err := staticContentStrategy(settings)
	if err != nil {
		return nil, err
	}
	threads, err := staticContentThreads(settings)
	if err != nil {
		return nil, err
	}
	if (strategy != "" || threads > 0) && len(locales) == 0 {
		return nil, errors.New("build.staticContent.strategy and threads require locales and themes")
	}
	result := make([]buildrunner.StaticContent, 0, len(locales)*len(themes))
	for _, locale := range locales {
		for _, theme := range themes {
			result = append(result, buildrunner.StaticContent{
				Locale:   locale,
				Theme:    theme,
				Strategy: strategy,
				Threads:  threads,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Locale == result[j].Locale {
			return result[i].Theme < result[j].Theme
		}
		return result[i].Locale < result[j].Locale
	})
	return result, nil
}

func staticContentStrategy(settings map[string]any) (string, error) {
	value, exists := settings["strategy"]
	if !exists || value == nil {
		return "", nil
	}
	strategy, ok := value.(string)
	if !ok || strategy == "" {
		return "", errors.New("build.staticContent.strategy must be quick, standard, or compact")
	}
	switch strategy {
	case "quick", "standard", "compact":
		return strategy, nil
	default:
		return "", fmt.Errorf("build.staticContent.strategy %q must be quick, standard, or compact", strategy)
	}
}

func staticContentThreads(settings map[string]any) (int, error) {
	value, exists := settings["threads"]
	if !exists || value == nil {
		return 0, nil
	}
	threads, err := positiveInt(value)
	if err != nil {
		return 0, fmt.Errorf("build.staticContent.threads must be a positive integer: %w", err)
	}
	return threads, nil
}

func positiveInt(value any) (int, error) {
	switch n := value.(type) {
	case int:
		if n < 1 {
			return 0, errors.New("must be >= 1")
		}
		return n, nil
	case int64:
		if n < 1 {
			return 0, errors.New("must be >= 1")
		}
		return int(n), nil
	case uint64:
		if n < 1 {
			return 0, errors.New("must be >= 1")
		}
		return int(n), nil
	default:
		return 0, errors.New("not an integer")
	}
}

func stringList(settings map[string]any, key string) ([]string, error) {
	value, exists := settings[key]
	if !exists {
		return nil, nil
	}
	entries, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("build.staticContent.%s must be a list", key)
	}
	result := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		text, ok := entry.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("build.staticContent.%s must contain non-empty strings", key)
		}
		if seen[text] {
			return nil, fmt.Errorf("build.staticContent.%s cannot contain duplicates", key)
		}
		seen[text] = true
		result = append(result, text)
	}
	sort.Strings(result)
	return result, nil
}
