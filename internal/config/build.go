package config

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

// BuildSpec contains only inputs that may affect the immutable application artifact.
type BuildSpec struct {
	SchemaVersion int                     `json:"schemaVersion" yaml:"schemaVersion"`
	Project       Project                 `json:"project" yaml:"project"`
	Application   Application             `json:"application" yaml:"application"`
	Build         Build                   `json:"build" yaml:"build"`
	Compatibility CompatibilityAssessment `json:"compatibility" yaml:"compatibility"`
}

// ResolveBuild resolves environment-independent artifact inputs.
func (f *File) ResolveBuild() (BuildSpec, error) {
	cfg, err := f.resolveProjectConfig()
	if err != nil {
		return BuildSpec{}, err
	}
	compatibility, err := validate(cfg)
	if err != nil {
		return BuildSpec{}, err
	}

	return BuildSpec{
		SchemaVersion: cfg.SchemaVersion,
		Project:       cfg.Project,
		Application:   cfg.Application,
		Build:         cfg.Build,
		Compatibility: compatibility,
	}, nil
}

// ProjectDefaults returns project-level runtime defaults without selecting an environment.
func (f *File) ProjectDefaults() (Defaults, error) {
	cfg, err := f.resolveProjectConfig()
	if err != nil {
		return Defaults{}, err
	}
	if _, err := validate(cfg); err != nil {
		return Defaults{}, err
	}
	return cfg.Defaults, nil
}

func (f *File) resolveProjectConfig() (Config, error) {
	if err := f.rejectEnvironmentBuildOverrides(); err != nil {
		return Config{}, err
	}

	value := map[string]any{}
	provenance := map[string]Provenance{}
	apply(value, builtInDefaults, "built-in defaults", "", provenance)
	apply(value, f.root, "project", "", provenance)
	data, err := yaml.Marshal(value)
	if err != nil {
		return Config{}, fmt.Errorf("encode build configuration: %w", err)
	}
	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode build configuration: %w", err)
	}
	return cfg, nil
}

func (f *File) rejectEnvironmentBuildOverrides() error {
	var violations []string
	for _, environment := range f.Environments() {
		for _, field := range []string{"project", "application", "build"} {
			if _, exists := f.envs[environment][field]; exists {
				violations = append(violations, environment+"."+field)
			}
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return errors.New("environment overrides cannot change immutable build inputs: " + strings.Join(violations, ", "))
}
