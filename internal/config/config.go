package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/secretref"
	"go.yaml.in/yaml/v4"
)

type File struct {
	doc          document
	root         map[string]any
	envs         map[string]map[string]any
	composerLock []byte
}

type ResolveOptions struct {
	Builtins        map[string]any
	Compatibility   map[string]any
	Presets         map[string]map[string]any
	Overrides       map[string]any
	PreviewIdentity *PreviewIdentity
	ComposerLock    []byte
}

var builtInDefaults = map[string]any{
	"application": map[string]any{"webRuntime": "nginx-fpm"},
}

func Load(data []byte) (*File, error) {
	var probe struct {
		SchemaVersion int `yaml:"schemaVersion"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if probe.SchemaVersion != 1 {
		if probe.SchemaVersion == 0 {
			return nil, errors.New("not a MageLift configuration: missing schemaVersion: 1 (PaaS files such as .magento.app.yaml / .platform.app.yaml are not valid --config input; use magelift init --from-acc or --from-upsun)")
		}
		return nil, fmt.Errorf("schemaVersion must be 1 (got %d)", probe.SchemaVersion)
	}

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
	provider := configuredTargetValue(f.root, f.envs, chain, opts.Overrides, "provider")
	runtime := configuredTargetValue(f.root, f.envs, chain, opts.Overrides, "runtime")
	value := map[string]any{}
	provenance := map[string]Provenance{}
	apply(value, builtInDefaults, "built-in defaults", "", provenance)
	apply(value, opts.Builtins, "explicit built-in defaults", "", provenance)
	apply(value, runtimeDefaults(applicationVersion(f.root, f.envs, chain)), "compatibility defaults", "", provenance)
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
		if usingDefaultPresets {
			p, ok = defaultProviderPreset(provider, runtime, preset, applicationVersion(f.root, f.envs, chain))
		}
		if !ok && len(opts.Presets) != 0 {
			return Effective{}, fmt.Errorf("unknown preset %q", preset)
		}
		if !usingDefaultPresets || hasProviderConfig(f.root, f.envs, chain, provider) {
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
	materializeContextualDefaults(value, provenance)
	normalizeDisabledProviderDefaults(value, provenance)
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
	if opts.PreviewIdentity != nil {
		identity := *opts.PreviewIdentity
		if err := identity.Validate(); err != nil {
			return Effective{}, fmt.Errorf("preview identity: %w", err)
		}
		cfg.PreviewIdentity = &identity
		provenance["previewIdentity"] = Provenance{Source: "derived preview identity"}
	}
	leaf := chain[len(chain)-1]
	emailExplicit := mapHasKey(f.envs[leaf], "email") || mapHasKey(opts.Overrides, "email")
	if applyPreviewEmailDefault(&cfg, emailExplicit) {
		provenance["email"] = Provenance{Source: "preview email default"}
		provenance["email.mode"] = Provenance{Source: "preview email default"}
	}
	normalizeCloudEmail(&cfg)
	compatibility, err := validate(cfg)
	if err != nil {
		return Effective{}, err
	}
	lock := opts.ComposerLock
	if len(lock) == 0 {
		lock = f.composerLock
	}
	if err := validateQueueModuleLock(cfg, lock); err != nil {
		return Effective{}, err
	}
	if cfg.Target.Provider == "aws" && !cfg.Compatibility.AllowUnsupported {
		if err := ValidateAWSServiceCompatibility(cfg); err != nil {
			return Effective{}, fmt.Errorf("invalid AWS service compatibility: %w", err)
		}
	}
	fingerprint, err := ResolvedFingerprint(cfg)
	if err != nil {
		return Effective{}, fmt.Errorf("fingerprint resolved configuration: %w", err)
	}
	return Effective{Config: cfg, Provenance: provenance, Compatibility: compatibility, Fingerprint: fingerprint}, nil
}

// materializeContextualDefaults resolves the small set of named defaults that
// depend on the selected environment class rather than only on provider,
// runtime, and preset. Keeping them in effective YAML prevents a provider
// planner from applying a safety choice that is invisible to provenance and
// architecture fingerprints.
func materializeContextualDefaults(value map[string]any, provenance map[string]Provenance) {
	provider := strings.TrimSpace(nestedString(value, "target", "provider"))
	preset := strings.TrimSpace(nestedString(value, "preset"))
	if preset == "" {
		preset = strings.TrimSpace(nestedString(value, "defaults", "preset"))
	}
	environmentClass := strings.TrimSpace(nestedString(value, "class"))

	switch provider {
	case "aws":
		if !hasNestedMap(value, "target", "aws") {
			return
		}
		// The owned-network path is the simple YAML path. Give it a stable,
		// inspectable network shape so a minimal target.aws block can reach the
		// planner without forcing every user to know AWS subnet prerequisites.
		// Adopted VPCs are different: their CIDR and subnet/AZ mapping are
		// operator-owned inputs and must remain explicit.
		if _, exists := nestedValue(value, []string{"target", "aws", "existing", "network"}); !exists {
			setIfAbsent(value, provenance, []string{"target", "aws", "vpcCidr"}, "10.42.0.0/16", "AWS network defaults")
			region := strings.TrimSpace(nestedString(value, "defaults", "region"))
			if region != "" {
				zoneCount := 2
				if preset == "high-availability" {
					zoneCount = 3
				}
				zones := make([]string, zoneCount)
				for index := range zones {
					zones[index] = region + string(rune('a'+index))
				}
				setIfAbsent(value, provenance, []string{"target", "aws", "availabilityZones"}, zones, "AWS network defaults")
			}
		}
		// RDS/Aurora uses production as the deletion-safety boundary. The
		// provider component already applies these values; materialize the
		// same policy here so effective YAML is authoritative.
		setIfAbsent(value, provenance, []string{"target", "aws", "catalog", "databaseDeletionProtection"}, environmentClass == "production", "environment class defaults")
		setIfAbsent(value, provenance, []string{"target", "aws", "catalog", "databaseDeleteAutomatedBackups"}, environmentClass != "production", "environment class defaults")
		if strings.TrimSpace(nestedString(value, "target", "aws", "natMode")) == "fck-nat" {
			// The fck-nat adapter has a deliberately small named default: the
			// ARM64 t4g.nano profile. Multi-AZ requires automatic replacement;
			// single-AZ remains a disposable single-instance boundary. Resolve
			// both choices here so the effective document and fingerprint do not
			// depend on an invisible network-component fallback.
			natTopology := strings.TrimSpace(nestedString(value, "target", "aws", "natTopology"))
			if natTopology == "" {
				natTopology = "single-az"
				if preset != "preview" {
					natTopology = "multi-az"
				}
				setIfAbsent(value, provenance, []string{"target", "aws", "natTopology"}, natTopology, "fck-nat defaults")
			}
			replacement := "none"
			if natTopology == "multi-az" {
				replacement = "auto-scaling"
			}
			setIfAbsent(value, provenance, []string{"target", "aws", "natReplacementMode"}, replacement, "fck-nat defaults")
			setIfAbsent(value, provenance, []string{"target", "aws", "natInstanceType"}, "t4g.nano", "fck-nat defaults")
		}
	case "gcp":
		if !hasNestedMap(value, "target", "gcp") {
			return
		}
		// Cloud Armor is enabled for durable presets and for production even
		// when production intentionally uses a disposable preview shape.
		cloudArmorEnabled := preset != "preview" || environmentClass == "production"
		setIfAbsent(value, provenance, []string{"target", "gcp", "enableCloudArmor"}, cloudArmorEnabled, contextualPresetSource(preset, environmentClass == "production" && preset == "preview"))
	}
}

func contextualPresetSource(preset string, productionPreview bool) string {
	if productionPreview {
		return "environment class defaults"
	}
	if preset != "" {
		return "preset " + preset
	}
	return "contextual defaults"
}

func setIfAbsent(value map[string]any, provenance map[string]Provenance, path []string, defaultValue any, source string) {
	if _, exists := nestedValue(value, path); exists {
		return
	}
	setNested(value, path, defaultValue)
	provenance[strings.Join(path, ".")] = Provenance{Source: source}
}

func setNested(values map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := values
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
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
	return hasProviderConfig(root, environments, chain, "aws")
}

func hasProviderConfig(root map[string]any, environments map[string]map[string]any, chain []string, provider string) bool {
	if provider == "" {
		return false
	}
	if hasNestedMap(root, "target", provider) {
		return true
	}
	for _, name := range chain {
		if hasNestedMap(environments[name], "target", provider) {
			return true
		}
	}
	return false
}

func configuredTargetValue(root map[string]any, environments map[string]map[string]any, chain []string, overrides map[string]any, field string) string {
	value := nestedString(root, "target", field)
	for _, name := range chain {
		if candidate := nestedString(environments[name], "target", field); candidate != "" {
			value = candidate
		}
	}
	if candidate := nestedString(overrides, "target", field); candidate != "" {
		value = candidate
	}
	return strings.TrimSpace(value)
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

// normalizeDisabledProviderDefaults removes only preset-owned schedule values
// that become meaningless when an operator explicitly disables the related
// managed backup service. Project and environment values remain in place so
// validation can report a contradictory advanced configuration instead of
// silently discarding it.
func normalizeDisabledProviderDefaults(value map[string]any, provenance map[string]Provenance) {
	clearPresetValuesWhenDisabled(value, provenance, "target.gcp.cloudSqlBackupEnabled", "target.gcp.cloudSqlBackupRetentionCount", "target.gcp.cloudSqlTransactionLogRetentionDays", "target.gcp.cloudSqlBackupStartTime", "target.gcp.cloudSqlBackupLocation")
	clearPresetValuesWhenDisabled(value, provenance, "target.scaleway.databaseBackupEnabled", "target.scaleway.databaseBackupFrequencyHours", "target.scaleway.databaseBackupRetentionDays", "target.scaleway.databaseBackupSameRegion")
	clearPresetValuesWhenExistingAWSNetwork(value, provenance)
}

// clearPresetValuesWhenExistingAWSNetwork keeps managed NAT defaults on the
// owned-network path only. An adopted VPC owns its own egress, so applying a
// preset NAT topology to it would be both misleading in effective YAML and a
// mutation attempt at the provider boundary. Explicit project or environment
// choices remain so the AWS planner can reject them instead of silently
// discarding operator intent.
func clearPresetValuesWhenExistingAWSNetwork(value map[string]any, provenance map[string]Provenance) {
	network, ok := nestedValue(value, []string{"target", "aws", "existing", "network"})
	if !ok || network == nil {
		return
	}
	if _, ok := network.(map[string]any); !ok {
		return
	}
	for _, path := range []string{
		"target.aws.natMode",
		"target.aws.natTopology",
		"target.aws.natReplacementMode",
		"target.aws.natInstanceType",
	} {
		entry, ok := provenance[path]
		if !ok || !strings.HasPrefix(entry.Source, "preset ") {
			continue
		}
		if deleteNested(value, strings.Split(path, ".")) {
			entry.Removed = true
			provenance[path] = entry
		}
	}
}

func clearPresetValuesWhenDisabled(value map[string]any, provenance map[string]Provenance, enabledPath string, dependentPaths ...string) {
	configured, ok := nestedValue(value, strings.Split(enabledPath, "."))
	if !ok {
		return
	}
	enabled, ok := configured.(bool)
	if !ok || enabled {
		return
	}
	for _, path := range dependentPaths {
		entry, ok := provenance[path]
		if !ok || !strings.HasPrefix(entry.Source, "preset ") {
			continue
		}
		if deleteNested(value, strings.Split(path, ".")) {
			entry.Removed = true
			provenance[path] = entry
		}
	}
}

func nestedValue(values map[string]any, path []string) (any, bool) {
	current := values
	for index, key := range path {
		value, ok := current[key]
		if !ok {
			return nil, false
		}
		if index == len(path)-1 {
			return value, true
		}
		next, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

func deleteNested(values map[string]any, path []string) bool {
	if len(path) == 0 {
		return false
	}
	current := values
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	if _, ok := current[path[len(path)-1]]; !ok {
		return false
	}
	delete(current, path[len(path)-1])
	return true
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
var phpExtensionName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var composerVersion = regexp.MustCompile(`^2\.\d+(?:\.\d+)?\+?$`)
var localServiceVersion = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]*$`)
var localPHPSettingName = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
var localCredentialEnvName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var magentoOverlayKey = regexp.MustCompile(`^(?:CONFIG__|MAGENTO_DC_)[A-Z0-9_]+$`)
var magentoSecretOverlayKey = regexp.MustCompile(`(?i)(?:password|crypt|secret|token|private[_-]?key)`)
var magentoFrontName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var qualityPatchID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var observabilityProvider = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var sensitiveObservabilityField = regexp.MustCompile(`(?i)(?:password|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)`)
var arm64NatInstanceType = regexp.MustCompile(`^(?:a1|[a-z0-9]+g[a-z0-9]*)\.[a-z0-9]+$`)
var gcpKubernetesMinor = regexp.MustCompile(`^1\.[0-9]+$`)
var awsClockWindow = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]-(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)
var awsMaintenanceWindow = regexp.MustCompile(`(?i)^(?:mon|tue|wed|thu|fri|sat|sun):(?:[01][0-9]|2[0-3]):[0-5][0-9]-(?:mon|tue|wed|thu|fri|sat|sun):(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

func validateAWSTarget(c Config) []string {
	if c.Target.AWS == nil {
		return nil
	}
	aws := c.Target.AWS
	var problems []string
	natMode := strings.TrimSpace(aws.NatMode)
	if natMode != "" && natMode != "nat-gateway" && natMode != "fck-nat" {
		problems = append(problems, "target.aws.natMode must be nat-gateway or fck-nat")
	}
	topology := strings.TrimSpace(aws.NatTopology)
	if topology != "" && topology != "single-az" && topology != "multi-az" {
		problems = append(problems, "target.aws.natTopology must be single-az or multi-az")
	}
	replacement := strings.TrimSpace(aws.NatReplacementMode)
	if replacement != "" && replacement != "none" && replacement != "auto-scaling" {
		problems = append(problems, "target.aws.natReplacementMode must be none or auto-scaling")
	}
	if natMode == "nat-gateway" && replacement == "auto-scaling" {
		problems = append(problems, "target.aws.natReplacementMode is only supported with fck-nat")
	}
	if instanceType := strings.TrimSpace(aws.NatInstanceType); instanceType != "" {
		if natMode != "fck-nat" {
			problems = append(problems, "target.aws.natInstanceType is only supported with fck-nat")
		} else if !arm64NatInstanceType.MatchString(instanceType) {
			problems = append(problems, "target.aws.natInstanceType must be an ARM64-compatible Graviton instance type such as t4g.nano")
		}
	}
	if aws.Existing.Network != nil && (natMode == "fck-nat" || topology != "" || replacement != "" || strings.TrimSpace(aws.NatInstanceType) != "") {
		problems = append(problems, "target.aws.existing.network owns egress; fck-nat and NAT topology/replacement settings cannot be selected for an existing VPC")
	}
	preset := strings.TrimSpace(c.Preset)
	if preset == "" {
		preset = strings.TrimSpace(c.Defaults.Preset)
	}
	if natMode == "fck-nat" && preset == "high-availability" {
		resolvedTopology := topology
		if resolvedTopology == "" {
			resolvedTopology = "multi-az"
		}
		resolvedReplacement := replacement
		if resolvedReplacement == "" && resolvedTopology == "multi-az" {
			resolvedReplacement = "auto-scaling"
		}
		if resolvedTopology != "multi-az" || resolvedReplacement != "auto-scaling" {
			problems = append(problems, "high-availability fck-nat requires multi-az topology with auto-scaling replacement")
		}
	}
	sort.Strings(problems)
	return problems
}

func validate(c Config) (CompatibilityAssessment, error) {
	var problems []string
	if c.PreviewIdentity != nil {
		if err := c.PreviewIdentity.Validate(); err != nil {
			problems = append(problems, "previewIdentity: "+err.Error())
		}
	}
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
	problems = append(problems, validateMagentoRuntime(c.Application)...)
	problems = append(problems, validateSingleTargetBlock(c)...)
	switch c.Target.Provider {
	case "aws":
		switch c.Target.Runtime {
		case "ecs-fargate", "eks":
		default:
			problems = append(problems, "target.runtime must be ecs-fargate or eks when provider is aws")
		}
		problems = append(problems, validateAWSTarget(c)...)
		problems = append(problems, validateAWSRuntimeCatalog(c)...)
	case "gcp":
		if c.Target.Runtime != "gke-autopilot" && c.Target.Runtime != "gke-standard" {
			problems = append(problems, "target.runtime must be gke-autopilot or gke-standard when provider is gcp")
		}
		if c.Target.GCP == nil {
			problems = append(problems, "target.gcp is required when provider is gcp")
		}
		// Target semantics validate provider-side (ValidateConfig); the
		// core keeps structural presence only.
	case "ovh":
		if c.Target.Runtime != "mks" {
			problems = append(problems, "target.runtime must be mks when provider is ovh")
		}
		if c.Target.OVH == nil || c.Target.OVH.ServiceName == "" {
			problems = append(problems, "target.ovh.serviceName is required when provider is ovh")
		}
		if c.Target.OVH != nil && (c.Target.OVH.DatabaseNodeCount < 0 || c.Target.OVH.ValkeyNodeCount < 0) {
			problems = append(problems, "target.ovh managed database node counts cannot be negative")
		}
		if c.Target.OVH != nil {
			if endpoint := strings.TrimSpace(c.Target.OVH.APIEndpoint); endpoint != "" && !validOVHAPIEndpoint(endpoint) {
				problems = append(problems, fmt.Sprintf("target.ovh.apiEndpoint %q is not supported; use ovh-eu, ovh-ca, or ovh-us", endpoint))
			}
			problems = append(problems, validateOVHBackupSettings(*c.Target.OVH)...)
			problems = append(problems, validateOVHManagedServiceSettings(*c.Target.OVH)...)
		}
	case "scaleway":
		if c.Target.Runtime != "kapsule" {
			problems = append(problems, "target.runtime must be kapsule when provider is scaleway")
		}
		if c.Target.Scaleway == nil || c.Target.Scaleway.ProjectID == "" {
			problems = append(problems, "target.scaleway.projectId is required when provider is scaleway")
		}
		if c.Target.Scaleway != nil && c.Target.Scaleway.CacheMode != "" && c.Target.Scaleway.CacheMode != "redis" {
			problems = append(problems, "target.scaleway.cacheMode must be redis when set")
		}
		if c.Target.Scaleway != nil && (c.Target.Scaleway.RedisClusterSize < 0 || c.Target.Scaleway.RedisClusterSize > 6) {
			problems = append(problems, "target.scaleway.redisClusterSize must be between 1 and 6")
		}
		if c.Target.Scaleway != nil {
			problems = append(problems, validateScalewayBackupSettings(*c.Target.Scaleway)...)
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
	problems = append(problems, validateProviderSecretReference("build.composer.credentials", c.Build.Composer.Credentials, c.Target.Provider)...)
	if c.Application.Edition == "commerce" && strings.TrimSpace(c.Build.Composer.Credentials) == "" {
		problems = append(problems, "build.composer.credentials is required for Adobe Commerce and must be a secret reference")
	}
	problems = append(problems, validateBuildRequirements(c.Build)...)
	problems = append(problems, validateBuildHooks(c.Build.Hooks)...)
	problems = append(problems, validateLocalRuntime(c.Local)...)
	problems = append(problems, validateCloudEmail(c)...)
	problems = append(problems, validateEdge(c)...)
	problems = append(problems, validateObservability(c)...)
	problems = append(problems, validateResilience(c)...)
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

func validateLocalRuntime(local LocalRuntime) []string {
	var problems []string
	validateService := func(name string, service LocalService, families ...string) {
		family := strings.TrimSpace(service.Family)
		version := strings.TrimSpace(service.Version)
		if family == "" && version == "" {
			return
		}
		if family == "" {
			problems = append(problems, name+".family is required when version is set")
		} else if !containsString(families, family) {
			problems = append(problems, fmt.Sprintf("%s.family must be one of %s", name, strings.Join(families, ", ")))
		}
		if version != "" && !localServiceVersion.MatchString(version) {
			problems = append(problems, name+".version must be a simple version identifier")
		}
	}
	validateService("local.database", local.Database, "mariadb", "mysql")
	validateService("local.cache", local.Cache, "redis", "valkey")
	validateService("local.search", local.Search, "elasticsearch", "opensearch")
	validateService("local.queue", local.Queue, "artemis", "database", "rabbitmq")
	validateService("local.webServer", local.WebServer, "nginx")
	validateService("local.webCache", local.WebCache, "none", "varnish")
	for key, value := range local.PHPSettings {
		if !localPHPSettingName.MatchString(key) {
			problems = append(problems, fmt.Sprintf("local.phpSettings key %q must be a lowercase PHP setting name", key))
		}
		if strings.ContainsAny(value, "\x00\r\n$=;#") {
			problems = append(problems, fmt.Sprintf("local.phpSettings[%q] contains a forbidden character", key))
		}
	}
	email := local.Email
	mode := strings.TrimSpace(email.Mode)
	if mode != "" && !containsString([]string{"disabled", "mailpit", "ses", "smtp"}, mode) {
		problems = append(problems, "local.email.mode must be disabled, mailpit, ses, or smtp")
	}
	if email.Port < 0 || email.Port > 65535 {
		problems = append(problems, "local.email.port must be between 1 and 65535 when set")
	}
	if mode == "smtp" && (strings.TrimSpace(email.Host) == "" || email.Port == 0) {
		problems = append(problems, "local.email.smtp requires host and port")
	}
	if strings.ContainsAny(email.Host+email.Username+email.From, "\x00\r\n$") {
		problems = append(problems, "local.email host, username, and from values contain a forbidden character")
	}
	if email.CredentialEnv != "" && !localCredentialEnvName.MatchString(email.CredentialEnv) {
		problems = append(problems, "local.email.credentialEnv must be an uppercase environment variable name")
	}
	sort.Strings(problems)
	return problems
}

func validateMagentoRuntime(application Application) []string {
	var problems []string
	runtime := application.Magento
	if frontName := strings.TrimSpace(runtime.FrontName); frontName != "" && !magentoFrontName.MatchString(frontName) {
		problems = append(problems, "application.magento.frontName must be a URL path segment without slashes")
	}
	if strings.ContainsAny(runtime.CookieDomain, "\x00\r\n$") {
		problems = append(problems, "application.magento.cookieDomain contains a forbidden character")
	}
	mode := strings.TrimSpace(runtime.Consumers.Mode)
	if mode != "" && !containsString([]string{"cron", "processes", "both"}, mode) {
		problems = append(problems, "application.magento.consumers.mode must be cron, processes, or both")
	}
	for _, origin := range runtime.CORSOrigins {
		if strings.TrimSpace(origin) == "" || strings.ContainsAny(origin, " \x00\r\n") {
			problems = append(problems, "application.magento.corsOrigins must be absolute origins without whitespace")
		}
	}
	if application.Mode != "headless" && (len(runtime.CORSOrigins) > 0 || strings.TrimSpace(runtime.StorefrontOrigin) != "") {
		problems = append(problems, "application.magento.corsOrigins and storefrontOrigin require application.mode headless")
	}
	transport := strings.TrimSpace(runtime.QueueTransport)
	if transport != "" && transport != "sqs" && transport != "pubsub" {
		problems = append(problems, "application.magento.queueTransport must be sqs or pubsub")
	}
	if transport == "sqs" || transport == "pubsub" {
		module := strings.TrimSpace(runtime.QueueModule)
		if module == "" || !strings.Contains(module, "/") {
			problems = append(problems, "MageLift: SQS/Pub/Sub requires a locked Composer Magento package in application.magento.queueModule; it is not a Magento queueMode")
		}
	}
	keys := make([]string, 0, len(runtime.Variables))
	for key := range runtime.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := runtime.Variables[key]
		if !magentoOverlayKey.MatchString(key) {
			problems = append(problems, fmt.Sprintf("application.magento.variables key %q must be a CONFIG__* or MAGENTO_DC_* identifier", key))
			continue
		}
		if magentoSecretOverlayKey.MatchString(key) {
			if _, err := secretref.Parse(value); err != nil {
				problems = append(problems, fmt.Sprintf("application.magento.variables[%q] must be a secret reference, not a literal", key))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func mapHasKey(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func applyPreviewEmailDefault(cfg *Config, emailExplicit bool) bool {
	if cfg == nil || emailExplicit || !strings.EqualFold(strings.TrimSpace(cfg.Class), "preview") {
		return false
	}
	cfg.Email = EmailConfig{Mode: "disabled"}
	return true
}

func normalizeCloudEmail(cfg *Config) {
	if cfg == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Email.Mode))
	cfg.Email.Mode = mode
}

func validateProviderSecretReference(field, value, provider string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	ref, err := secretref.Parse(value)
	if err != nil {
		return []string{field + " must be a valid secret reference, not plaintext"}
	}
	switch provider {
	case "aws":
		if ref.Kind != secretref.SecretsManager && ref.Kind != secretref.ParameterStore {
			return []string{field + " for aws must use aws-secrets-manager:// or ssm://"}
		}
	case "gcp":
		if ref.Kind != secretref.GCPSecretManager {
			return []string{field + " for gcp must use gcp-secret-manager://"}
		}
	}
	return nil
}

func validateCloudEmail(c Config) []string {
	email := c.Email
	mode := strings.ToLower(strings.TrimSpace(email.Mode))
	provider := strings.ToLower(strings.TrimSpace(c.Target.Provider))
	managed := email.Managed != nil
	var problems []string
	if mode != "" && !containsString([]string{"disabled", "smtp", "ses", "tem", "ovh"}, mode) {
		problems = append(problems, "email.mode must be disabled, smtp, ses, tem, or ovh")
	}
	if managed && mode != "ses" && mode != "tem" && mode != "ovh" {
		problems = append(problems, "email.managed requires mode ses, tem, or ovh")
	}
	if email.Port < 0 || email.Port > 65535 {
		problems = append(problems, "email.port must be between 1 and 65535 when set")
	}
	if strings.ContainsAny(email.Host+email.Username+email.From, "\x00\r\n$") {
		problems = append(problems, "email host, username, and from values contain a forbidden character")
	}
	byoSet := strings.TrimSpace(email.Host) != "" || email.Port != 0 || strings.TrimSpace(email.Username) != "" || strings.TrimSpace(email.Credential) != ""
	var managedDomain, managedZone, managedAccount string
	if email.Managed != nil {
		managedDomain = strings.TrimSpace(email.Managed.Domain)
		managedZone = strings.TrimSpace(email.Managed.HostedZoneID)
		managedAccount = strings.TrimSpace(email.Managed.Account)
	}
	switch mode {
	case "ses":
		if !managed {
			if strings.TrimSpace(email.Host) == "" || email.Port == 0 || strings.TrimSpace(email.Username) == "" || strings.TrimSpace(email.Credential) == "" {
				problems = append(problems, "email.ses requires host, port, username, and a credential secret reference")
			}
			break
		}
		if provider != "aws" {
			problems = append(problems, "email.ses managed mode requires target.provider aws")
		}
		if managedDomain == "" || managedZone == "" {
			problems = append(problems, "email.ses managed mode requires managed.domain and managed.hostedZoneId")
		}
		if byoSet {
			problems = append(problems, "email.ses managed mode cannot be combined with explicit host, port, username, or credential")
		}
		if managedAccount != "" {
			problems = append(problems, "email.managed.account applies only to ovh mode")
		}
	case "tem":
		problems = append(problems, "email.tem managed mode is not implemented for alpha; use email.mode smtp with an explicit relay")
	case "ovh":
		problems = append(problems, "email.ovh managed mode is not implemented for alpha; use email.mode smtp with an explicit relay")
	case "smtp":
		if strings.TrimSpace(email.Host) == "" || email.Port == 0 {
			problems = append(problems, "email.smtp requires host and port")
		}
	}
	problems = append(problems, validateProviderSecretReference("email.credential", email.Credential, c.Target.Provider)...)
	return problems
}

func effectiveScalewayRegion(c Config) string {
	if c.Target.Scaleway != nil {
		if region := strings.TrimSpace(c.Target.Scaleway.Region); region != "" {
			return region
		}
	}
	return strings.TrimSpace(c.Defaults.Region)
}

// validateSingleTargetBlock enforces the generic target-block rule: at most
// one target.* block may be set, and the block must match the provider.
// Provider-specific semantics validate provider-side; this stays structural.
func validateSingleTargetBlock(c Config) []string {
	var set []string
	if c.Target.AWS != nil {
		set = append(set, "target.aws")
	}
	if c.Target.GCP != nil {
		set = append(set, "target.gcp")
	}
	if c.Target.OVH != nil {
		set = append(set, "target.ovh")
	}
	if c.Target.Scaleway != nil {
		set = append(set, "target.scaleway")
	}
	if len(set) > 1 {
		sort.Strings(set)
		return []string{fmt.Sprintf("only one target block may be set; found %s", strings.Join(set, ", "))}
	}
	if len(set) == 1 {
		want := map[string]string{"aws": "target.aws", "gcp": "target.gcp", "ovh": "target.ovh", "scaleway": "target.scaleway"}[c.Target.Provider]
		if want != "" && set[0] != want {
			return []string{fmt.Sprintf("%s is not valid when provider is %s", set[0], c.Target.Provider)}
		}
	}
	return nil
}
func validateScalewayBackupSettings(target ScalewayTarget) []string {
	var problems []string
	if target.DatabaseBackupFrequency != nil && *target.DatabaseBackupFrequency < 1 {
		problems = append(problems, "target.scaleway.databaseBackupFrequencyHours must be at least 1")
	}
	if target.DatabaseBackupRetention != nil && *target.DatabaseBackupRetention < 1 {
		problems = append(problems, "target.scaleway.databaseBackupRetentionDays must be at least 1")
	}
	if target.DatabaseBackupEnabled != nil && !*target.DatabaseBackupEnabled && (target.DatabaseBackupFrequency != nil || target.DatabaseBackupRetention != nil || target.DatabaseBackupSameRegion != nil) {
		problems = append(problems, "target.scaleway database backup schedule settings require databaseBackupEnabled")
	}
	return problems
}

func validateOVHBackupSettings(target OVHTarget) []string {
	var problems []string
	for name, value := range map[string]string{
		"databaseBackupTime": target.DatabaseBackupTime,
		"valkeyBackupTime":   target.ValkeyBackupTime,
	} {
		if value != "" && !validClockTime(value) {
			problems = append(problems, fmt.Sprintf("target.ovh.%s must use HH:MM", name))
		}
	}
	for name, regions := range map[string][]string{
		"databaseBackupRegions": target.DatabaseBackupRegions,
		"valkeyBackupRegions":   target.ValkeyBackupRegions,
	} {
		if len(regions) > 2 {
			problems = append(problems, fmt.Sprintf("target.ovh.%s may contain at most two regions", name))
		}
		for _, region := range regions {
			if strings.TrimSpace(region) == "" || strings.ContainsAny(region, "\r\n\x00") {
				problems = append(problems, fmt.Sprintf("target.ovh.%s must contain non-empty region identifiers without control characters", name))
			}
		}
	}
	return problems
}

func validateOVHManagedServiceSettings(target OVHTarget) []string {
	var problems []string
	if target.DatabaseVersion != "" && target.DatabaseVersion != "8.0" && target.DatabaseVersion != "8.4" {
		problems = append(problems, fmt.Sprintf("target.ovh.databaseVersion %q is not currently documented; use 8.0 or 8.4", target.DatabaseVersion))
	}
	if target.ValkeyVersion != "" && target.ValkeyVersion != "7.2" && target.ValkeyVersion != "8.0" && target.ValkeyVersion != "8.1" && target.ValkeyVersion != "9.0" && target.ValkeyVersion != "9.1" {
		problems = append(problems, fmt.Sprintf("target.ovh.valkeyVersion %q is not currently documented or catalog-admitted; use 7.2, 8.0, 8.1, 9.0, or 9.1", target.ValkeyVersion))
	}
	if target.DatabasePlan != "" {
		minNodes, maxNodes, ok := ovhMySQLPlanBounds(target.DatabasePlan)
		if !ok {
			problems = append(problems, fmt.Sprintf("target.ovh.databasePlan %q is not supported; use discovery, essential, business, production, enterprise, or advanced", target.DatabasePlan))
		} else if target.DatabaseNodeCount > 0 && target.DatabaseNodeCount < minNodes {
			problems = append(problems, fmt.Sprintf("target.ovh.databaseNodeCount %d is below the %s plan minimum of %d", target.DatabaseNodeCount, target.DatabasePlan, minNodes))
		} else if target.DatabaseNodeCount > maxNodes {
			problems = append(problems, fmt.Sprintf("target.ovh.databaseNodeCount %d exceeds the %s plan limit of %d", target.DatabaseNodeCount, target.DatabasePlan, maxNodes))
		}
	}
	if target.ValkeyPlan != "" {
		minNodes, maxNodes, ok := ovhValkeyPlanBounds(target.ValkeyPlan)
		if !ok {
			problems = append(problems, fmt.Sprintf("target.ovh.valkeyPlan %q is not supported; use discovery, essential, business, or production", target.ValkeyPlan))
		} else if target.ValkeyNodeCount > 0 && target.ValkeyNodeCount < minNodes {
			problems = append(problems, fmt.Sprintf("target.ovh.valkeyNodeCount %d is below the %s plan minimum of %d", target.ValkeyNodeCount, target.ValkeyPlan, minNodes))
		} else if target.ValkeyNodeCount > maxNodes {
			problems = append(problems, fmt.Sprintf("target.ovh.valkeyNodeCount %d exceeds the %s plan limit of %d", target.ValkeyNodeCount, target.ValkeyPlan, maxNodes))
		}
	}
	seenZones := make(map[string]struct{}, len(target.Zones))
	for index, zone := range target.Zones {
		trimmed := strings.TrimSpace(zone)
		if trimmed == "" {
			problems = append(problems, fmt.Sprintf("target.ovh.zones[%d] must not be empty", index))
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seenZones[key]; exists {
			problems = append(problems, fmt.Sprintf("target.ovh.zones contains duplicate availability zone %q", zone))
			continue
		}
		seenZones[key] = struct{}{}
	}
	return problems
}

func validOVHAPIEndpoint(endpoint string) bool {
	switch endpoint {
	case "ovh-eu", "ovh-ca", "ovh-us":
		return true
	default:
		return false
	}
}

func ovhMySQLPlanBounds(plan string) (int, int, bool) {
	switch plan {
	case "discovery", "essential":
		return 1, 1, true
	case "business", "production":
		return 2, 2, true
	case "enterprise", "advanced":
		return 3, 3, true
	default:
		return 0, 0, false
	}
}

func ovhValkeyPlanBounds(plan string) (int, int, bool) {
	switch plan {
	case "discovery", "essential":
		return 1, 1, true
	case "business", "production":
		return 2, 2, true
	default:
		return 0, 0, false
	}
}

func validClockTime(value string) bool {
	parsed, err := time.Parse("15:04", value)
	return err == nil && parsed.Format("15:04") == value
}

func validateBuildRequirements(build Build) []string {
	var problems []string
	seen := make(map[string]struct{}, len(build.Extensions))
	for _, extension := range build.Extensions {
		if !phpExtensionName.MatchString(extension) {
			problems = append(problems, fmt.Sprintf("build.extensions entry %q must be a lowercase PHP extension name", extension))
			continue
		}
		if _, exists := seen[extension]; exists {
			problems = append(problems, fmt.Sprintf("build.extensions cannot contain duplicate %q", extension))
		}
		seen[extension] = struct{}{}
	}
	if build.Composer.Version != "" && !composerVersion.MatchString(build.Composer.Version) {
		problems = append(problems, fmt.Sprintf("build.composer.version %q must be a Composer 2 major.minor, major.minor.patch, or minimum version with +", build.Composer.Version))
	}
	seenPatches := map[string]struct{}{}
	for _, id := range build.QualityPatches {
		id = strings.TrimSpace(id)
		if !qualityPatchID.MatchString(id) {
			problems = append(problems, fmt.Sprintf("build.qualityPatches entry %q must be a Quality Patch ID", id))
			continue
		}
		if _, exists := seenPatches[id]; exists {
			problems = append(problems, fmt.Sprintf("build.qualityPatches cannot contain duplicate %q", id))
		}
		seenPatches[id] = struct{}{}
	}
	sort.Strings(problems)

	return problems
}

func validateEdge(c Config) []string {
	edge := c.Edge
	var problems []string
	mode := strings.TrimSpace(edge.Mode)
	if mode == "" {
		switch {
		case edge.NativeProvider != "" && edge.ExternalProvider == "fastly":
			mode = "both"
		case edge.NativeProvider != "":
			mode = "native"
		case edge.ExternalProvider != "":
			mode = "external"
		default:
			mode = "none"
		}
	}
	switch mode {
	case "none", "native", "external", "both":
	default:
		problems = append(problems, "edge.mode must be one of none, native, external, or both")
	}
	if (mode == "native" || mode == "both") && strings.TrimSpace(edge.NativeProvider) == "" {
		problems = append(problems, "edge.nativeProvider is required for native or both edge mode")
	}
	if c.Target.Provider == "ovh" && (mode == "native" || mode == "both") && strings.EqualFold(strings.TrimSpace(edge.NativeProvider), "cdn") {
		problems = append(problems, "OVH: native CDN is unavailable (no MKS CDN adapter); use Fastly or the MKS load balancer")
	}
	if (mode == "external" || mode == "both") && strings.TrimSpace(edge.ExternalProvider) == "" {
		problems = append(problems, "edge.externalProvider is required for external or both edge mode")
	}
	if mode == "none" && (edge.ExternalProvider != "" || edge.ServiceID != "" || edge.TokenSecret != "" || len(edge.Domains) != 0 || edge.TLS || edge.VCLRef != "" || edge.PurgeOnDeploy || edge.NativeProvider != "" || edge.Health != nil) {
		problems = append(problems, "edge fields require a non-none edge mode")
	}
	if edge.ExternalProvider == "fastly" {
		if strings.TrimSpace(edge.ServiceID) == "" {
			problems = append(problems, "edge.serviceId is required for Fastly")
		}
		if strings.TrimSpace(edge.TokenSecret) == "" {
			problems = append(problems, "edge.tokenSecret is required for Fastly and must be a secret reference")
		} else if _, err := secretref.Parse(edge.TokenSecret); err != nil {
			problems = append(problems, "edge.tokenSecret must be a valid secret reference, not plaintext")
		}
	}
	if strings.ContainsAny(edge.VCLRef, "\r\n") {
		problems = append(problems, "edge.vclRef must reference reviewed VCL or policy content, not contain the content inline")
	}
	for name, value := range map[string]string{
		"tlsMode": edge.TLSMode, "dnsMode": edge.DNSMode, "originHealthRef": edge.OriginHealthRef,
		"cachePolicyRef": edge.CachePolicyRef, "purgePolicyRef": edge.PurgePolicyRef, "wafPolicyRef": edge.WAFPolicyRef,
		"failoverPolicyRef": edge.FailoverPolicyRef, "ownershipMarker": edge.OwnershipMarker,
	} {
		if strings.ContainsAny(value, "\r\n") {
			problems = append(problems, fmt.Sprintf("edge.%s must not contain line breaks", name))
		}
	}
	if edge.ExternalProvider == "fastly" && (mode == "external" || mode == "both") && edge.Health == nil {
		problems = append(problems, "edge.health is required for Fastly edge verification")
	}
	if edge.Health != nil {
		health := edge.Health
		for name, value := range map[string]string{
			"health.originUrl":     health.OriginURL,
			"health.originHost":    health.OriginHost,
			"health.expectedCname": health.ExpectedCNAME,
			"health.routePath":     health.RoutePath,
		} {
			if strings.ContainsAny(value, "\r\n\x00") {
				problems = append(problems, fmt.Sprintf("edge.%s must not contain control characters", name))
			}
		}
		if health.OriginURL != "" {
			parsed, err := url.Parse(strings.TrimSpace(health.OriginURL))
			if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				problems = append(problems, "edge.health.originUrl must be an http or https URL")
			}
		}
		if health.RoutePath != "" && !strings.HasPrefix(health.RoutePath, "/") {
			problems = append(problems, "edge.health.routePath must start with /")
		}
		if health.ExpectedStatus < 0 || health.ExpectedStatus > 599 {
			problems = append(problems, "edge.health.expectedStatus must be between 100 and 599 when set")
		} else if health.ExpectedStatus > 0 && health.ExpectedStatus < 100 {
			problems = append(problems, "edge.health.expectedStatus must be between 100 and 599 when set")
		}
		if health.RouteTimeoutSeconds < 0 || health.RouteTimeoutSeconds > 3600 {
			problems = append(problems, "edge.health.routeTimeoutSeconds must be between 1 and 3600 when set")
		}
		if health.RoutePollSeconds < 0 || health.RoutePollSeconds > 300 {
			problems = append(problems, "edge.health.routePollSeconds must be between 1 and 300 when set")
		}
		if edge.ExternalProvider == "fastly" && (mode == "external" || mode == "both") {
			if strings.TrimSpace(health.ExpectedCNAME) == "" {
				problems = append(problems, "edge.health.expectedCname is required for Fastly edge verification")
			}
		}
	}
	if mode != "none" && strings.TrimSpace(edge.OriginHealthRef) == "" {
		problems = append(problems, "edge.originHealthRef is required for every enabled edge mode")
	}
	return problems
}

func validateObservability(c Config) []string {
	observability := c.Observability
	var problems []string
	nativeProvider := strings.TrimSpace(observability.NativeProvider)
	externalProvider := strings.TrimSpace(observability.ExternalProvider)
	if nativeProvider == "" && externalProvider == "" {
		if observability.NativeReference != "" || observability.Endpoint != "" || observability.ServiceName != "" || observability.Environment != "" || observability.DataResidency != "" || observability.Logs || observability.Metrics || observability.Traces || len(observability.Signals) != 0 || len(observability.Labels) != 0 || len(observability.CredentialReferences) != 0 || observability.RetentionDays != 0 || observability.SamplingRatio != 0 || observability.RedactionPolicyRef != "" || len(observability.AlertReferences) != 0 || len(observability.Alerts) != 0 || len(observability.Dashboards) != 0 || len(observability.SLOs) != 0 {
			return []string{"observability fields require a nativeProvider or externalProvider"}
		}
		return nil
	}
	if observability.NativeReference != "" && (nativeProvider == "" || nativeProvider == "none") {
		problems = append(problems, "observability.nativeReference requires an active nativeProvider")
	}
	for name, candidate := range map[string]string{"nativeProvider": nativeProvider, "externalProvider": externalProvider} {
		if candidate != "" && !observabilityProvider.MatchString(candidate) {
			problems = append(problems, fmt.Sprintf("observability.%s must use lowercase letters, digits, dots, or hyphens", name))
		}
	}
	if nativeProvider == "xray" || nativeProvider == "aws-xray" {
		problems = append(problems, "observability.nativeProvider xray is typed unavailable until an ObservabilityAdapter plugin is registered; the EKS IAM policy snippet is not a Magento cell")
	}
	for name, value := range map[string]string{
		"endpoint":        observability.Endpoint,
		"serviceName":     observability.ServiceName,
		"environment":     observability.Environment,
		"dataResidency":   observability.DataResidency,
		"nativeReference": observability.NativeReference,
	} {
		if strings.ContainsAny(value, "\r\n") {
			problems = append(problems, fmt.Sprintf("observability.%s must not contain line breaks", name))
		}
	}
	if !observability.Logs && !observability.Metrics && !observability.Traces {
		if len(observability.Signals) == 0 {
			problems = append(problems, "observability must enable logs, metrics, or traces")
		}
	}
	if observability.RetentionDays < 0 || observability.RetentionDays > 3650 {
		problems = append(problems, "observability.retentionDays must be between 0 and 3650")
	}
	if observability.SamplingRatio < 0 || observability.SamplingRatio > 1 {
		problems = append(problems, "observability.samplingRatio must be between 0 and 1")
	}
	for _, signal := range observability.Signals {
		if !observabilityProvider.MatchString(signal) {
			problems = append(problems, fmt.Sprintf("observability.signals value %q must use lowercase letters, digits, dots, or hyphens", signal))
		}
	}
	for _, reference := range observability.CredentialReferences {
		if strings.TrimSpace(reference) == "" {
			problems = append(problems, "observability.credentialReferences must contain secret references")
			continue
		}
		if _, err := secretref.Parse(reference); err != nil {
			problems = append(problems, fmt.Sprintf("observability.credentialReferences value must be a valid secret reference: %v", err))
		}
	}
	for name, value := range observability.Labels {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") || sensitiveObservabilityField.MatchString(name) {
			problems = append(problems, "observability.labels keys must be non-empty, non-sensitive, and must not contain line breaks")
		}
		if strings.ContainsAny(value, "\r\n") || strings.Contains(value, "://") || sensitiveObservabilityField.MatchString(value) {
			problems = append(problems, fmt.Sprintf("observability.labels[%q] must not contain line breaks or secret-like values", name))
		}
	}
	seenAlerts := make(map[string]struct{}, len(observability.Alerts))
	for _, alert := range observability.Alerts {
		if err := validateObservabilityAlert(alert); err != "" {
			problems = append(problems, err)
		}
		if _, exists := seenAlerts[alert.ID]; exists {
			problems = append(problems, fmt.Sprintf("observability.alerts contains duplicate id %q", alert.ID))
		}
		seenAlerts[alert.ID] = struct{}{}
	}
	seenDashboards := make(map[string]struct{}, len(observability.Dashboards))
	for _, dashboard := range observability.Dashboards {
		if err := validateObservabilityDashboard(dashboard); err != "" {
			problems = append(problems, err)
		}
		if _, exists := seenDashboards[dashboard.ID]; exists {
			problems = append(problems, fmt.Sprintf("observability.dashboards contains duplicate id %q", dashboard.ID))
		}
		seenDashboards[dashboard.ID] = struct{}{}
	}
	seenSLOs := make(map[string]struct{}, len(observability.SLOs))
	for _, slo := range observability.SLOs {
		if err := validateObservabilitySLO(slo); err != "" {
			problems = append(problems, err)
		}
		if _, exists := seenSLOs[slo.ID]; exists {
			problems = append(problems, fmt.Sprintf("observability.slos contains duplicate id %q", slo.ID))
		}
		seenSLOs[slo.ID] = struct{}{}
	}
	sort.Strings(problems)
	return problems
}

func validateObservabilityDashboard(dashboard ObservabilityDashboard) string {
	if dashboard.ID == "" || !observabilityProvider.MatchString(dashboard.ID) || strings.TrimSpace(dashboard.Owner) == "" {
		return fmt.Sprintf("observability.dashboards[%q] requires lowercase id and owner", dashboard.ID)
	}
	if len(dashboard.Signals) == 0 {
		return fmt.Sprintf("observability.dashboards[%q] requires at least one signal", dashboard.ID)
	}
	seen := make(map[string]struct{}, len(dashboard.Signals))
	for _, signal := range dashboard.Signals {
		if !observabilityProvider.MatchString(signal) {
			return fmt.Sprintf("observability.dashboards[%q] has invalid signal %q", dashboard.ID, signal)
		}
		if _, exists := seen[signal]; exists {
			return fmt.Sprintf("observability.dashboards[%q] contains duplicate signal %q", dashboard.ID, signal)
		}
		seen[signal] = struct{}{}
	}
	return ""
}

func validateObservabilityAlert(alert ObservabilityAlert) string {
	if alert.ID == "" || !observabilityProvider.MatchString(alert.ID) || alert.Signal == "" || !observabilityProvider.MatchString(alert.Signal) {
		return fmt.Sprintf("observability.alerts[%q] requires lowercase id and signal", alert.ID)
	}
	switch alert.Severity {
	case "info", "warning", "critical":
	default:
		return fmt.Sprintf("observability.alerts[%q].severity is invalid", alert.ID)
	}
	switch alert.Operator {
	case "gt", "gte", "lt", "lte", "eq":
	default:
		return fmt.Sprintf("observability.alerts[%q].operator is invalid", alert.ID)
	}
	if alert.Threshold < 0 || alert.WindowSeconds <= 0 || strings.TrimSpace(alert.Owner) == "" || strings.TrimSpace(alert.DeduplicationKey) == "" {
		return fmt.Sprintf("observability.alerts[%q] requires a non-negative threshold, positive window, owner, and deduplication key", alert.ID)
	}
	if err := validateHTTPSURL(alert.RunbookURL); err != nil {
		return fmt.Sprintf("observability.alerts[%q].runbookUrl must be an HTTPS URL", alert.ID)
	}
	return ""
}

func validateObservabilitySLO(slo ObservabilitySLO) string {
	if slo.ID == "" || !observabilityProvider.MatchString(slo.ID) || slo.Signal == "" || !observabilityProvider.MatchString(slo.Signal) {
		return fmt.Sprintf("observability.slos[%q] requires lowercase id and signal", slo.ID)
	}
	if slo.Target <= 0 || slo.Target > 1 || slo.WindowSeconds <= 0 || strings.TrimSpace(slo.Owner) == "" || strings.TrimSpace(slo.ErrorBudgetPolicy) == "" {
		return fmt.Sprintf("observability.slos[%q] requires a target in (0,1], positive window, owner, and error-budget policy", slo.ID)
	}
	if err := validateHTTPSURL(slo.RunbookURL); err != nil {
		return fmt.Sprintf("observability.slos[%q].runbookUrl must be an HTTPS URL", slo.ID)
	}
	return ""
}

func validateHTTPSURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("URL must use HTTPS")
	}
	return nil
}

func validateResilience(c Config) []string {
	resilience := c.Resilience
	emptyPolicy := resilience.ProfileID == "" && resilience.AvailabilityTarget == "" && resilience.RPOSeconds == 0 && resilience.RTOSeconds == 0 && resilience.RetentionDays == 0 && resilience.RecoveryScope == "" && resilience.FailoverOwner == "" && resilience.FencingPolicy == "" && resilience.Projection == nil && len(resilience.RecoveryDestinations) == 0 && len(resilience.DataClasses) == 0
	if emptyPolicy && strings.TrimSpace(resilience.DataRegion) == "" {
		return nil
	}
	var problems []string
	if emptyPolicy {
		return append(problems, validateDataResidency(c)...)
	}
	for name, value := range map[string]string{"profileId": resilience.ProfileID, "availabilityTarget": resilience.AvailabilityTarget, "recoveryScope": resilience.RecoveryScope, "failoverOwner": resilience.FailoverOwner, "fencingPolicy": resilience.FencingPolicy} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Sprintf("resilience.%s is required", name))
		}
	}
	if resilience.RPOSeconds <= 0 || resilience.RTOSeconds <= 0 || resilience.RetentionDays <= 0 {
		problems = append(problems, "resilience.rpoSeconds, resilience.rtoSeconds, and resilience.retentionDays must be positive")
	}
	problems = append(problems, validateResilienceProjection(c, resilience.Projection)...)
	seenDestinations := make(map[string]struct{}, len(resilience.RecoveryDestinations))
	for _, destination := range resilience.RecoveryDestinations {
		switch destination {
		case "same-region", "same-region-isolated", "alternate-region", "alternate-provider":
		default:
			problems = append(problems, fmt.Sprintf("resilience.recoveryDestinations contains invalid value %q", destination))
		}
		if _, exists := seenDestinations[destination]; exists {
			problems = append(problems, fmt.Sprintf("resilience.recoveryDestinations contains duplicate value %q", destination))
		}
		seenDestinations[destination] = struct{}{}
	}
	seenClasses := make(map[string]struct{}, len(resilience.DataClasses))
	for _, dataClass := range resilience.DataClasses {
		if dataClass.Name == "" || dataClass.SourceOfTruth == "" || dataClass.BackupMethod == "" || dataClass.RestoreMethod == "" || dataClass.IntegrityMethod == "" || dataClass.LossSemantics == "" || dataClass.OwnershipMarker == "" {
			problems = append(problems, fmt.Sprintf("resilience.dataClasses[%q] is incomplete", dataClass.Name))
		}
		if dataClass.RetentionDays <= 0 {
			problems = append(problems, fmt.Sprintf("resilience.dataClasses[%q].retentionDays must be positive", dataClass.Name))
		}
		if _, exists := seenClasses[dataClass.Name]; exists {
			problems = append(problems, fmt.Sprintf("resilience.dataClasses contains duplicate value %q", dataClass.Name))
		}
		seenClasses[dataClass.Name] = struct{}{}
	}
	problems = append(problems, validateDataResidency(c)...)
	sort.Strings(problems)
	return problems
}

func validateDataResidency(c Config) []string {
	declared := strings.TrimSpace(c.Resilience.DataRegion)
	if declared == "" {
		return nil
	}
	if strings.ContainsAny(declared, "\r\n\x00") {
		return []string{"resilience.dataRegion must not contain control characters"}
	}
	var problems []string
	region := strings.TrimSpace(c.Defaults.Region)
	if region != "" && !regionHonorsResidency(declared, region) {
		problems = append(problems, fmt.Sprintf("resilience.dataRegion %q refuses defaults.region %q", declared, region))
	}
	// Residency is a cross-cutting core policy over already-parsed data, not
	// target validation: the provider never receives dataRegion, so only
	// the core can enforce it. Kept deliberately during the GCP slimming.
	if c.Target.GCP != nil {
		location := strings.TrimSpace(c.Target.GCP.CloudSQLBackupLocation)
		if location != "" && !regionHonorsResidency(declared, location) {
			problems = append(problems, fmt.Sprintf("resilience.dataRegion %q refuses target.gcp.cloudSqlBackupLocation %q", declared, location))
		}
	}
	return problems
}

func regionHonorsResidency(declared, actual string) bool {
	declared = strings.ToLower(strings.TrimSpace(declared))
	actual = strings.ToLower(strings.TrimSpace(actual))
	if declared == "" || actual == "" || declared == actual {
		return true
	}
	switch declared {
	case "eu", "europe":
		return strings.HasPrefix(actual, "eu-") || strings.HasPrefix(actual, "europe-")
	case "us", "america":
		return strings.HasPrefix(actual, "us-")
	default:
		return strings.HasPrefix(actual, declared)
	}
}

func validateResilienceProjection(c Config, projection *ResilienceProjection) []string {
	if projection == nil {
		return nil
	}
	var problems []string
	for name, value := range map[string]string{
		"runtime": projection.Runtime, "cluster": projection.Cluster, "service": projection.Service,
		"task": projection.Task, "namespace": projection.Namespace, "workload": projection.Workload,
		"container": projection.Container,
	} {
		if strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Sprintf("resilience.projection.%s must not contain control characters", name))
		}
	}
	switch projection.Runtime {
	case "ecs":
		if c.Target.Provider != "aws" || c.Target.Runtime != "ecs-fargate" {
			problems = append(problems, "resilience.projection.runtime ecs requires target.provider aws and target.runtime ecs-fargate")
		}
		if strings.TrimSpace(projection.Cluster) == "" || strings.TrimSpace(projection.Container) == "" {
			problems = append(problems, "resilience.projection ecs requires cluster and container")
		}
		if (strings.TrimSpace(projection.Service) == "") == (strings.TrimSpace(projection.Task) == "") {
			problems = append(problems, "resilience.projection ecs requires exactly one of service or task")
		}
		if projection.Namespace != "" || projection.Workload != "" {
			problems = append(problems, "resilience.projection namespace and workload are only valid for Kubernetes")
		}
	case "kubernetes":
		if c.Target.Provider == "aws" && c.Target.Runtime != "eks" || c.Target.Provider == "gcp" && c.Target.Runtime != "gke-autopilot" && c.Target.Runtime != "gke-standard" || c.Target.Provider == "scaleway" && c.Target.Runtime != "kapsule" || c.Target.Provider == "ovh" && c.Target.Runtime != "mks" {
			problems = append(problems, "resilience.projection.runtime kubernetes requires a Kubernetes target runtime")
		}
		if strings.TrimSpace(projection.Workload) == "" {
			problems = append(problems, "resilience.projection kubernetes requires workload")
		}
		if projection.Cluster != "" || projection.Service != "" || projection.Task != "" {
			problems = append(problems, "resilience.projection cluster, service, and task are only valid for ECS")
		}
	default:
		problems = append(problems, "resilience.projection.runtime must be ecs or kubernetes")
	}
	return problems
}

func validateAWSRuntimeCatalog(c Config) []string {
	if c.Target.AWS == nil {
		return nil
	}
	var problems []string
	catalog := c.Target.AWS.Catalog
	problems = append(problems, validateAWSManagedDurability(catalog)...)
	eksSet := catalog.EKS.KubernetesVersion != "" ||
		catalog.EKS.ComputeMode != "" || catalog.EKS.NodeInstanceType != "" || catalog.EKS.NodeAMI != "" || catalog.EKS.NodeMinSize > 0 || catalog.EKS.NodeDesiredSize > 0 || catalog.EKS.NodeMaxSize > 0 || len(catalog.EKS.FargateNamespaces) > 0 ||
		catalog.EKS.CPURequest != "" || catalog.EKS.MemoryRequest != "" ||
		catalog.EKS.DesiredWebReplicas > 0 || catalog.EKS.QueueConsumerCount > 0 ||
		catalog.EKS.SearchMode != "" || catalog.EKS.SearchReplicas > 0 ||
		catalog.EKS.QueueMode != "" || catalog.EKS.QueueReplicas > 0
	// Preset defaults always populate Fargate desiredCount for AWS projects; treat
	// CPU/memory as the signal that the ECS catalog was intentionally sized.
	fargateSized := catalog.Fargate.CPU > 0 || catalog.Fargate.MemoryMiB > 0
	switch c.Target.Runtime {
	case "ecs-fargate":
		if eksSet {
			problems = append(problems, "target.aws.catalog.eks requires runtime eks")
		}
		if mode := strings.TrimSpace(catalog.Fargate.ComputeMode); mode != "" && !containsString([]string{"fargate", "fargate-spot", "ec2-asg", "managed-instances"}, mode) {
			problems = append(problems, "target.aws.catalog.fargate.computeMode must be fargate, fargate-spot, ec2-asg, or managed-instances")
		}
		if catalog.Fargate.MinCapacity < 0 || catalog.Fargate.MaxCapacity < 0 || (catalog.Fargate.MaxCapacity > 0 && catalog.Fargate.MinCapacity > catalog.Fargate.MaxCapacity) {
			problems = append(problems, "target.aws.catalog.fargate capacity bounds are invalid")
		}
	case "eks":
		if fargateSized {
			problems = append(problems, "target.aws.catalog.fargate requires runtime ecs-fargate")
		}
		if mode := strings.TrimSpace(catalog.EKS.ComputeMode); mode != "" && !containsString([]string{"auto-mode", "managed-node-groups", "self-managed", "fargate"}, mode) {
			problems = append(problems, "target.aws.catalog.eks.computeMode must be auto-mode, managed-node-groups, self-managed, or fargate")
		}
		if catalog.EKS.NodeMinSize < 0 || catalog.EKS.NodeDesiredSize < 0 || catalog.EKS.NodeMaxSize < 0 || (catalog.EKS.NodeMaxSize > 0 && catalog.EKS.NodeMinSize > catalog.EKS.NodeMaxSize) || (catalog.EKS.NodeDesiredSize > 0 && catalog.EKS.NodeDesiredSize < catalog.EKS.NodeMinSize) || (catalog.EKS.NodeMaxSize > 0 && catalog.EKS.NodeDesiredSize > catalog.EKS.NodeMaxSize) {
			problems = append(problems, "target.aws.catalog.eks node capacity bounds are invalid")
		}
		if mode := strings.TrimSpace(catalog.EKS.ComputeMode); mode == "self-managed" && !strings.HasPrefix(strings.TrimSpace(catalog.EKS.NodeAMI), "ami-") {
			problems = append(problems, "target.aws.catalog.eks.nodeAmi is required and must be a pinned ami-* value for self-managed nodes")
		}
		if mode := strings.TrimSpace(catalog.EKS.ComputeMode); mode != "" && mode != "self-managed" && catalog.EKS.NodeAMI != "" {
			problems = append(problems, "target.aws.catalog.eks.nodeAmi is only supported for self-managed nodes")
		}
		if mode := strings.TrimSpace(catalog.EKS.ComputeMode); (mode == "" || mode == "auto-mode" || mode == "fargate") && (catalog.EKS.NodeInstanceType != "" || catalog.EKS.NodeMinSize > 0 || catalog.EKS.NodeDesiredSize > 0 || catalog.EKS.NodeMaxSize > 0) {
			problems = append(problems, "target.aws.catalog.eks node capacity settings require managed-node-groups or self-managed mode")
		}
		if mode := strings.TrimSpace(catalog.EKS.ComputeMode); mode != "fargate" && len(catalog.EKS.FargateNamespaces) > 0 {
			problems = append(problems, "target.aws.catalog.eks.fargateNamespaces requires fargate compute mode")
		}
		if mode := strings.TrimSpace(catalog.EKS.ComputeMode); mode == "fargate" && (catalog.EKS.SearchMode == "opensearch" || catalog.EKS.QueueMode == "rabbitmq") {
			problems = append(problems, "target.aws.catalog.eks fargate mode cannot host persistent OpenSearch or RabbitMQ workloads")
		}
	}
	return problems
}

func validateAWSManagedDurability(catalog AWSCatalog) []string {
	var problems []string
	if value := strings.TrimSpace(catalog.DatabaseBackupWindow); value != "" && !awsClockWindow.MatchString(value) {
		problems = append(problems, "target.aws.catalog.databaseBackupWindow must use HH:MM-HH:MM in UTC")
	}
	if value := strings.TrimSpace(catalog.DatabaseMaintenanceWindow); value != "" && !awsMaintenanceWindow.MatchString(value) {
		problems = append(problems, "target.aws.catalog.databaseMaintenanceWindow must use ddd:HH:MM-ddd:HH:MM in UTC")
	}
	if catalog.CacheSnapshotRetentionLimit != nil && (*catalog.CacheSnapshotRetentionLimit < 0 || *catalog.CacheSnapshotRetentionLimit > 35) {
		problems = append(problems, "target.aws.catalog.cacheSnapshotRetentionLimit must be between 0 and 35 days")
	}
	if value := strings.TrimSpace(catalog.CacheSnapshotWindow); value != "" {
		if !awsClockWindow.MatchString(value) {
			problems = append(problems, "target.aws.catalog.cacheSnapshotWindow must use HH:MM-HH:MM in UTC")
		}
		if catalog.CacheSnapshotRetentionLimit == nil || *catalog.CacheSnapshotRetentionLimit == 0 {
			problems = append(problems, "target.aws.catalog.cacheSnapshotWindow requires cacheSnapshotRetentionLimit between 1 and 35")
		}
	}
	return problems
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// ValidateAWSServiceCompatibility checks the concrete AWS service versions
// before a provider planner can create or mutate resources. The AWS service
// catalog is intentionally separate from Adobe's generic requirements because
// AWS exposes provider-specific engine strings and availability constraints.
func ValidateAWSServiceCompatibility(c Config) error {
	if c.Target.AWS == nil {
		return nil
	}
	versionLine := strings.SplitN(c.Application.Version, "-", 2)[0]
	policy, ok := map[string]struct {
		openSearchPrefix string
		valkeyPrefixes   []string
	}{
		"2.4.9": {openSearchPrefix: "OpenSearch_3", valkeyPrefixes: []string{"8", "9"}},
		"2.4.8": {openSearchPrefix: "OpenSearch_3", valkeyPrefixes: []string{"8"}},
		"2.4.7": {openSearchPrefix: "OpenSearch_", valkeyPrefixes: []string{"8"}},
		"2.4.6": {openSearchPrefix: "OpenSearch_", valkeyPrefixes: []string{"8"}},
	}[versionLine]
	if !ok {
		return fmt.Errorf("AWS service compatibility is not cataloged for Magento %s", versionLine)
	}
	versions := c.Target.AWS.Catalog.Versions
	preset := strings.TrimSpace(c.Preset)
	if preset == "" {
		preset = strings.TrimSpace(c.Defaults.Preset)
	}
	searchMode := strings.TrimSpace(c.Target.AWS.Catalog.SearchMode)
	if searchMode == "" {
		if preset == "preview" {
			searchMode = "disabled"
		} else {
			searchMode = "provisioned"
		}
	}
	queueMode := strings.TrimSpace(c.Target.AWS.Catalog.QueueMode)
	if queueMode == "" {
		if preset == "preview" {
			queueMode = "db"
		} else {
			queueMode = "ecs-rabbitmq"
		}
	}
	if searchMode != "disabled" {
		searchCompatible := strings.HasPrefix(versions.OpenSearch, policy.openSearchPrefix)
		if versionLine == "2.4.7" || versionLine == "2.4.6" {
			searchCompatible = strings.HasPrefix(versions.OpenSearch, "OpenSearch_2") || strings.HasPrefix(versions.OpenSearch, "OpenSearch_3")
		}
		if !searchCompatible {
			return fmt.Errorf("Magento %s requires an Adobe-listed AWS OpenSearch 2 or 3 version", versionLine)
		}
	}
	if !hasAnyPrefix(versions.Valkey, policy.valkeyPrefixes) {
		return fmt.Errorf("Magento %s requires an Adobe-listed AWS Valkey version", versionLine)
	}
	if queueMode == "amazon-mq" || queueMode == "ecs-rabbitmq" {
		if !strings.HasPrefix(versions.RabbitMQ, "3.13") && !strings.HasPrefix(versions.RabbitMQ, "4.2") {
			return fmt.Errorf("Magento %s requires RabbitMQ 3.13 or 4.2 for queueMode %s", versionLine, queueMode)
		}
	}
	if queueMode == "amazon-mq" && strings.HasPrefix(versions.RabbitMQ, "4.2") {
		instanceType := strings.TrimSpace(c.Target.AWS.Catalog.RabbitMQ.InstanceType)
		if instanceType != "" && !strings.HasPrefix(instanceType, "mq.m7g.") {
			return errors.New("AWS MQ RabbitMQ 4.2 requires an mq.m7g instance type")
		}
	}
	if c.Target.AWS.Catalog.DatabaseEngine == "rds-mysql" {
		if strings.TrimSpace(versions.MySQL) == "" || (!strings.HasPrefix(versions.MySQL, "8.0.") && !strings.HasPrefix(versions.MySQL, "8.4.")) {
			return fmt.Errorf("Magento %s requires an AWS RDS MySQL 8.0 or 8.4 engine", versionLine)
		}
		return nil
	}
	if c.Target.AWS.Catalog.DatabaseEngine == "rds-mariadb" {
		mariaDBPrefixes := map[string][]string{
			"2.4.9": {"11.8"},
			"2.4.8": {"11.4", "11.8"},
			"2.4.7": {"10.11", "11.8"},
			"2.4.6": {"10.11"},
		}[versionLine]
		if !hasAnyPrefix(versions.MariaDB, mariaDBPrefixes) {
			return fmt.Errorf("Magento %s requires an Adobe-listed AWS RDS MariaDB engine (%s)", versionLine, strings.Join(mariaDBPrefixes, " or "))
		}
		return nil
	}
	if !strings.Contains(versions.AuroraMySQL, ".3.12") && !strings.Contains(versions.AuroraMySQL, ".3.11") {
		return fmt.Errorf("Magento %s requires an AWS Aurora MySQL 3.11 or 3.12 engine", versionLine)
	}
	return nil
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
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
