package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/acourtiol/magelift/internal/secretref"
	"go.yaml.in/yaml/v4"
)

type File struct {
	doc  document
	root map[string]any
	envs map[string]map[string]any
}

type ResolveOptions struct {
	Builtins      map[string]any
	Compatibility map[string]any
	Presets       map[string]map[string]any
	Overrides     map[string]any
}

var builtInDefaults = map[string]any{
	"application": map[string]any{"webRuntime": "nginx-fpm"},
}

func Load(data []byte) (*File, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var doc document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("config must contain exactly one YAML document")
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode config values: %w", err)
	}
	envs := make(map[string]map[string]any)
	if value, ok := raw["environments"]; ok {
		entries, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("environments must be a map")
		}
		for name, value := range entries {
			overlay, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("environment %q must be a map", name)
			}
			envs[name] = overlay
		}
	}
	delete(raw, "environments")
	return &File{doc: doc, root: raw, envs: envs}, nil
}

func (f *File) Environments() []string {
	names := make([]string, 0, len(f.envs))
	for name := range f.envs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EnvironmentForBranch returns the environment explicitly mapped to branch.
// Branch mappings are exact to keep selection predictable across local and CI runs.
func (f *File) EnvironmentForBranch(branch string) (string, bool, error) {
	if branch == "" {
		return "", false, nil
	}
	var match string
	for _, name := range f.Environments() {
		environment := f.doc.Environments[name]
		for _, configuredBranch := range environment.Branches {
			if configuredBranch != branch {
				continue
			}
			if match != "" && match != name {
				return "", false, fmt.Errorf("branch %q maps to multiple environments: %q and %q", branch, match, name)
			}
			match = name
		}
	}
	return match, match != "", nil
}

func (f *File) Resolve(environment string, opts ResolveOptions) (Effective, error) {
	if environment == "" {
		return Effective{}, errors.New("environment is required")
	}
	chain, err := f.environmentChain(environment)
	if err != nil {
		return Effective{}, err
	}
	usingDefaultPresets := len(opts.Presets) == 0
	if usingDefaultPresets {
		opts.Presets = DefaultResolveOptions().Presets
	}
	value := map[string]any{}
	provenance := map[string]Provenance{}
	apply(value, builtInDefaults, "built-in defaults", "", provenance)
	apply(value, opts.Builtins, "explicit built-in defaults", "", provenance)
	if hasAWSConfig(f.root, f.envs, chain) {
		apply(value, compatibilityDefaults(applicationVersion(f.root, f.envs, chain)), "compatibility defaults", "", provenance)
	}
	apply(value, opts.Compatibility, "compatibility defaults", "", provenance)
	preset := f.doc.Defaults.Preset
	for _, name := range chain {
		if p, ok := f.envs[name]["preset"].(string); ok {
			preset = p
		}
	}
	if preset != "" {
		p, ok := opts.Presets[preset]
		if !ok && len(opts.Presets) != 0 {
			return Effective{}, fmt.Errorf("unknown preset %q", preset)
		}
		if !usingDefaultPresets || hasAWSConfig(f.root, f.envs, chain) {
			apply(value, p, "preset "+preset, "", provenance)
		}
	}
	apply(value, f.root, "project", "", provenance)
	for _, name := range chain {
		overlay := cloneMap(f.envs[name])
		delete(overlay, "inherits")
		apply(value, overlay, "environment "+name, "", provenance)
	}
	apply(value, opts.Overrides, "CLI override", "", provenance)
	data, err := yaml.Marshal(value)
	if err != nil {
		return Effective{}, err
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Effective{}, fmt.Errorf("decode effective config: %w", err)
	}
	compatibility, err := validate(cfg)
	if err != nil {
		return Effective{}, err
	}
	return Effective{Config: cfg, Provenance: provenance, Compatibility: compatibility}, nil
}

func applicationVersion(root map[string]any, environments map[string]map[string]any, chain []string) string {
	version := nestedString(root, "application", "version")
	for _, name := range chain {
		if candidate := nestedString(environments[name], "application", "version"); candidate != "" {
			version = candidate
		}
	}
	return version
}

func hasAWSConfig(root map[string]any, environments map[string]map[string]any, chain []string) bool {
	if hasNestedMap(root, "target", "aws") {
		return true
	}
	for _, name := range chain {
		if hasNestedMap(environments[name], "target", "aws") {
			return true
		}
	}
	return false
}

func nestedString(values map[string]any, path ...string) string {
	current := values
	for index, key := range path {
		value, ok := current[key]
		if !ok {
			return ""
		}
		if index == len(path)-1 {
			result, _ := value.(string)
			return result
		}
		next, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func hasNestedMap(values map[string]any, path ...string) bool {
	current := values
	for index, key := range path {
		value, ok := current[key]
		if !ok {
			return false
		}
		if index == len(path)-1 {
			_, ok := value.(map[string]any)
			return ok
		}
		next, ok := value.(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	return false
}

func (f *File) environmentChain(name string) ([]string, error) {
	var chain []string
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(current string) error {
		if visiting[current] {
			return fmt.Errorf("environment inheritance cycle at %q", current)
		}
		if visited[current] {
			return nil
		}
		env, ok := f.envs[current]
		if !ok {
			return fmt.Errorf("unknown environment %q", current)
		}
		visiting[current] = true
		if parent, ok := env["inherits"].(string); ok && parent != "" {
			if err := visit(parent); err != nil {
				return err
			}
		}
		delete(visiting, current)
		visited[current] = true
		chain = append(chain, current)
		return nil
	}
	return chain, visit(name)
}

func apply(dst, src map[string]any, source, prefix string, provenance map[string]Provenance) {
	for key, incoming := range src {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if incoming == nil {
			delete(dst, key)
			provenance[path] = Provenance{Source: source, Removed: true}
			continue
		}
		if incomingMap, ok := incoming.(map[string]any); ok {
			current, ok := dst[key].(map[string]any)
			if !ok {
				current = map[string]any{}
				dst[key] = current
			}
			apply(current, incomingMap, source, path, provenance)
			continue
		}
		dst[key] = incoming
		provenance[path] = Provenance{Source: source}
	}
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

var extensionNamespace = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*\.)+[a-z0-9][a-z0-9-]*$`)
var lifecycleID = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

func validate(c Config) (CompatibilityAssessment, error) {
	var problems []string
	if c.SchemaVersion != 1 {
		problems = append(problems, "schemaVersion must be 1")
	}
	if c.Project.Name == "" {
		problems = append(problems, "project.name is required")
	}
	if c.Application.Edition != "open-source" && c.Application.Edition != "commerce" {
		problems = append(problems, "application.edition must be open-source or commerce")
	}
	if c.Application.Mode != "integrated" && c.Application.Mode != "headless" {
		problems = append(problems, "application.mode must be integrated or headless")
	}
	if c.Application.WebRuntime != "nginx-fpm" && c.Application.WebRuntime != "frankenphp-classic" {
		problems = append(problems, "application.webRuntime must be nginx-fpm or frankenphp-classic")
	}
	switch c.Target.Provider {
	case "aws":
		switch c.Target.Runtime {
		case "ecs-fargate", "eks-autopilot":
		default:
			problems = append(problems, "target.runtime must be ecs-fargate or eks-autopilot when provider is aws")
		}
		if c.Target.GCP != nil {
			problems = append(problems, "target.gcp is not valid when provider is aws")
		}
		if c.Target.OVH != nil {
			problems = append(problems, "target.ovh is not valid when provider is aws")
		}
		if c.Target.Scaleway != nil {
			problems = append(problems, "target.scaleway is not valid when provider is aws")
		}
		problems = append(problems, validateAWSRuntimeCatalog(c)...)
	case "gcp":
		if c.Target.Runtime != "gke-autopilot" {
			problems = append(problems, "target.runtime must be gke-autopilot when provider is gcp")
		}
		if c.Target.AWS != nil {
			problems = append(problems, "target.aws is not valid when provider is gcp")
		}
		if c.Target.OVH != nil {
			problems = append(problems, "target.ovh is not valid when provider is gcp")
		}
		if c.Target.Scaleway != nil {
			problems = append(problems, "target.scaleway is not valid when provider is gcp")
		}
		if c.Target.GCP == nil || c.Target.GCP.Project == "" {
			problems = append(problems, "target.gcp.project is required when provider is gcp")
		}
	case "ovh":
		if c.Target.Runtime != "mks" {
			problems = append(problems, "target.runtime must be mks when provider is ovh")
		}
		if c.Target.AWS != nil {
			problems = append(problems, "target.aws is not valid when provider is ovh")
		}
		if c.Target.GCP != nil {
			problems = append(problems, "target.gcp is not valid when provider is ovh")
		}
		if c.Target.Scaleway != nil {
			problems = append(problems, "target.scaleway is not valid when provider is ovh")
		}
		if c.Target.OVH == nil || c.Target.OVH.ServiceName == "" {
			problems = append(problems, "target.ovh.serviceName is required when provider is ovh")
		}
	case "scaleway":
		if c.Target.Runtime != "kapsule" {
			problems = append(problems, "target.runtime must be kapsule when provider is scaleway")
		}
		if c.Target.AWS != nil {
			problems = append(problems, "target.aws is not valid when provider is scaleway")
		}
		if c.Target.GCP != nil {
			problems = append(problems, "target.gcp is not valid when provider is scaleway")
		}
		if c.Target.OVH != nil {
			problems = append(problems, "target.ovh is not valid when provider is scaleway")
		}
		if c.Target.Scaleway == nil || c.Target.Scaleway.ProjectID == "" {
			problems = append(problems, "target.scaleway.projectId is required when provider is scaleway")
		}
		if c.Target.Scaleway != nil && c.Target.Scaleway.CacheMode != "" && c.Target.Scaleway.CacheMode != "redis" {
			problems = append(problems, "target.scaleway.cacheMode must be redis when set")
		}
	default:
		problems = append(problems, "target.provider must be aws, gcp, ovh, or scaleway")
	}
	if c.Defaults.Preset != "preview" && c.Defaults.Preset != "standard" && c.Defaults.Preset != "high-availability" {
		problems = append(problems, "defaults.preset must be preview, standard, or high-availability")
	}
	if c.Preset != "" && c.Preset != "preview" && c.Preset != "standard" && c.Preset != "high-availability" {
		problems = append(problems, "environment preset must be preview, standard, or high-availability")
	}
	if credential := c.Build.Composer.Credentials; credential != "" {
		ref, err := secretref.Parse(credential)
		if err != nil {
			problems = append(problems, "build.composer.credentials must be a valid secret reference, not plaintext")
		} else {
			switch c.Target.Provider {
			case "aws":
				if ref.Kind != secretref.SecretsManager && ref.Kind != secretref.ParameterStore {
					problems = append(problems, "build.composer.credentials for aws must use aws-secrets-manager:// or ssm://")
				}
			case "gcp":
				if ref.Kind != secretref.GCPSecretManager {
					// Accepted at config time; build resolution wires the GCP provider later.
				} else {
					problems = append(problems, "build.composer.credentials for gcp must use gcp-secret-manager://")
				}
			}
		}
	}
	problems = append(problems, validateBuildHooks(c.Build.Hooks)...)
	for namespace := range c.Extensions {
		if !extensionNamespace.MatchString(namespace) {
			problems = append(problems, fmt.Sprintf("extensions key %q must be a namespaced identifier such as vendor.feature", namespace))
		}
	}
	compatibility, compatibilityProblem := assessCompatibility(c)
	if compatibilityProblem != "" {
		problems = append(problems, compatibilityProblem)
	}
	if len(problems) != 0 {
		return compatibility, errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return compatibility, nil
}

func validateAWSRuntimeCatalog(c Config) []string {
	if c.Target.AWS == nil {
		return nil
	}
	catalog := c.Target.AWS.Catalog
	eksSet := catalog.EKS.CPURequest != "" || catalog.EKS.MemoryRequest != "" || catalog.EKS.DesiredWebReplicas > 0 || catalog.EKS.QueueConsumerCount > 0
	// Preset defaults always populate Fargate desiredCount for AWS projects; treat
	// CPU/memory as the signal that the ECS catalog was intentionally sized.
	fargateSized := catalog.Fargate.CPU > 0 || catalog.Fargate.MemoryMiB > 0
	switch c.Target.Runtime {
	case "ecs-fargate":
		if eksSet {
			return []string{"target.aws.catalog.eks requires runtime eks-autopilot"}
		}
	case "eks-autopilot":
		if fargateSized {
			return []string{"target.aws.catalog.fargate requires runtime ecs-fargate"}
		}
	}
	return nil
}

func validateBuildHooks(hooks map[string]BuildHook) []string {
	if len(hooks) == 0 {
		return nil
	}
	var problems []string
	for id, hook := range hooks {
		if !lifecycleID.MatchString(id) {
			problems = append(problems, fmt.Sprintf("build.hooks key %q must be a stable lifecycle ID", id))
		}
		if hook.Phase != "validate" && hook.Phase != "build" && hook.Phase != "package" {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.phase must be validate, build, or package", id))
		}
		if hook.Relationship != "before" && hook.Relationship != "after" && hook.Relationship != "replace" && hook.Relationship != "disable" {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.relationship must be before, after, replace, or disable", id))
		}
		if !lifecycleID.MatchString(hook.Target) {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.target must be a stable lifecycle ID", id))
		}
		if hook.Relationship == "disable" {
			if hook.Command != nil {
				problems = append(problems, fmt.Sprintf("build.hooks.%s.disable cannot define a command", id))
			}
		} else if hook.Command == nil {
			problems = append(problems, fmt.Sprintf("build.hooks.%s requires a command", id))
		} else {
			if hook.Command.Executable != "composer" && hook.Command.Executable != "magento" {
				problems = append(problems, fmt.Sprintf("build.hooks.%s.command.executable must be composer or magento", id))
			}
			for _, argument := range hook.Command.Arguments {
				if argument == "" || strings.ContainsRune(argument, '\x00') {
					problems = append(problems, fmt.Sprintf("build.hooks.%s.command.arguments must contain non-empty strings without null bytes", id))
					break
				}
			}
		}
		for _, dependency := range hook.Dependencies {
			if !lifecycleID.MatchString(dependency) {
				problems = append(problems, fmt.Sprintf("build.hooks.%s.dependencies must contain stable lifecycle IDs", id))
			}
		}
		timeout := hook.Timeout
		if timeout == 0 {
			timeout = 300
		}
		if timeout < 1 || timeout > 7200 {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.timeoutSeconds must be between 1 and 7200", id))
		}
		attempts := hook.Retries.MaxAttempts
		if attempts == 0 {
			attempts = 1
		}
		if attempts < 1 || attempts > 5 {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.retries.maxAttempts must be between 1 and 5", id))
		}
		if hook.Retries.DelaySeconds < 0 || hook.Retries.DelaySeconds > 3600 {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.retries.delaySeconds must be between 0 and 3600", id))
		}
		if attempts > 1 && !hook.Retries.Idempotent {
			problems = append(problems, fmt.Sprintf("build.hooks.%s retries require idempotent: true", id))
		}
		if hook.Failure != "" && hook.Failure != "abort" && hook.Failure != "continue" {
			problems = append(problems, fmt.Sprintf("build.hooks.%s.failure must be abort or continue", id))
		}
	}
	sort.Strings(problems)

	return problems
}
