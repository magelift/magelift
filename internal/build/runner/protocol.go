package runner

import (
	"errors"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const ProtocolVersion = 1

type Stage string

const (
	StagePrepare  Stage = "prepare"
	StageFinalize Stage = "finalize"
)

type Request struct {
	ProtocolVersion int              `json:"protocolVersion"`
	Stage           Stage            `json:"stage"`
	Prepare         *PrepareRequest  `json:"prepare,omitempty"`
	Finalize        *FinalizeRequest `json:"finalize,omitempty"`
}

type Application struct {
	Edition    string `json:"edition"`
	Version    string `json:"version"`
	Mode       string `json:"mode"`
	WebRuntime string `json:"webRuntime"`
}

type InputFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type StaticContent struct {
	Locale string `json:"locale"`
	Theme  string `json:"theme"`
}

type LifecycleHook struct {
	ID           string       `json:"id"`
	Phase        string       `json:"phase"`
	Relationship string       `json:"relationship"`
	Target       string       `json:"target"`
	Command      *HookCommand `json:"command,omitempty"`
	Dependencies []string     `json:"dependencies,omitempty"`
	Timeout      int          `json:"timeoutSeconds,omitempty"`
	Retries      HookRetries  `json:"retries,omitempty"`
	Failure      string       `json:"failure,omitempty"`
}

type HookCommand struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments,omitempty"`
}

type HookRetries struct {
	MaxAttempts  int  `json:"maxAttempts,omitempty"`
	DelaySeconds int  `json:"delaySeconds,omitempty"`
	Idempotent   bool `json:"idempotent,omitempty"`
}

// PrepareRequest contains only inputs that affect artifact bytes. Credentials
// and environment runtime values are deliberately absent from the protocol.
type PrepareRequest struct {
	RepositoryRoot      string          `json:"repositoryRoot"`
	SourceRevision      string          `json:"sourceRevision"`
	Application         Application     `json:"application"`
	PHPVersion          string          `json:"phpVersion"`
	CompatibilityStatus string          `json:"compatibilityStatus"`
	InputFiles          []InputFile     `json:"inputFiles"`
	StaticContent       []StaticContent `json:"staticContent"`
	LifecycleHooks      []LifecycleHook `json:"lifecycleHooks,omitempty"`
}

type FinalizeRequest struct {
	PreparedArtifact string `json:"preparedArtifact"`
	SourceRevision   string `json:"sourceRevision"`
	ImageDigest      string `json:"imageDigest"`
}

type Response struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Stage           Stage             `json:"stage"`
	Prepare         *PrepareResponse  `json:"prepare,omitempty"`
	Finalize        *FinalizeResponse `json:"finalize,omitempty"`
}

type FileChecksum struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type PrepareResponse struct {
	PreparedArtifact            string         `json:"preparedArtifact"`
	PHPVersion                  string         `json:"phpVersion"`
	PHPExtensions               []string       `json:"phpExtensions"`
	EnabledModules              []string       `json:"enabledModules"`
	Checksums                   []FileChecksum `json:"checksums"`
	RequiredRuntimeCapabilities []string       `json:"requiredRuntimeCapabilities"`
}

type FinalizeResponse struct {
	ImageDigest    string `json:"imageDigest"`
	ManifestPath   string `json:"manifestPath"`
	ManifestSHA256 string `json:"manifestSha256"`
}

var sha256Digest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var sha256Checksum = regexp.MustCompile(`^[a-f0-9]{64}$`)
var sourceRevision = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
var stableID = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

func (request Request) Validate() error {
	var problems []error
	if request.ProtocolVersion != ProtocolVersion {
		problems = append(problems, fmt.Errorf("unsupported build runner protocol version %d", request.ProtocolVersion))
	}
	switch request.Stage {
	case StagePrepare:
		if request.Prepare == nil || request.Finalize != nil {
			problems = append(problems, errors.New("prepare stage requires only a prepare payload"))
		} else {
			problems = append(problems, request.Prepare.validate())
		}
	case StageFinalize:
		if request.Finalize == nil || request.Prepare != nil {
			problems = append(problems, errors.New("finalize stage requires only a finalize payload"))
		} else {
			problems = append(problems, request.Finalize.validate())
		}
	default:
		problems = append(problems, fmt.Errorf("invalid build runner stage %q", request.Stage))
	}
	return errors.Join(problems...)
}

func (request PrepareRequest) validate() error {
	var problems []error
	if !filepath.IsAbs(request.RepositoryRoot) {
		problems = append(problems, errors.New("repositoryRoot must be an absolute path"))
	}
	if !sourceRevision.MatchString(request.SourceRevision) {
		problems = append(problems, errors.New("sourceRevision must be a lowercase commit hash"))
	}
	if request.Application.Edition != "open-source" && request.Application.Edition != "commerce" {
		problems = append(problems, errors.New("application edition must be open-source or commerce"))
	}
	if request.Application.Version == "" || request.PHPVersion == "" || request.Application.WebRuntime == "" {
		problems = append(problems, errors.New("application version, PHP version, and web runtime are required"))
	} else if request.Application.WebRuntime != "nginx-fpm" && request.Application.WebRuntime != "frankenphp-classic" {
		problems = append(problems, errors.New("web runtime must be nginx-fpm or frankenphp-classic"))
	}
	if request.CompatibilityStatus != "supported" && request.CompatibilityStatus != "unsupported-allowed" {
		problems = append(problems, errors.New("compatibilityStatus must be supported or unsupported-allowed"))
	}
	if request.Application.Mode != "integrated" && request.Application.Mode != "headless" {
		problems = append(problems, errors.New("application mode must be integrated or headless"))
	}
	if len(request.InputFiles) == 0 {
		problems = append(problems, errors.New("at least one immutable build input file is required"))
	}
	seen := make(map[string]struct{}, len(request.InputFiles))
	for _, input := range request.InputFiles {
		problems = append(problems, validateRelativePath("build input", input.Path))
		if !sha256Checksum.MatchString(input.SHA256) {
			problems = append(problems, fmt.Errorf("build input %q has an invalid SHA-256 checksum", input.Path))
		}
		if _, exists := seen[input.Path]; exists {
			problems = append(problems, fmt.Errorf("duplicate build input path %q", input.Path))
		}
		seen[input.Path] = struct{}{}
	}
	staticContent := make(map[string]struct{}, len(request.StaticContent))
	for _, content := range request.StaticContent {
		if strings.TrimSpace(content.Locale) == "" || strings.TrimSpace(content.Theme) == "" {
			problems = append(problems, errors.New("static content locale and theme are required"))
		}
		key := content.Locale + "\x00" + content.Theme
		if _, exists := staticContent[key]; exists {
			problems = append(problems, fmt.Errorf("duplicate static content entry for locale %q and theme %q", content.Locale, content.Theme))
		}
		staticContent[key] = struct{}{}
	}
	seenHooks := make(map[string]struct{}, len(request.LifecycleHooks))
	for _, hook := range request.LifecycleHooks {
		if !stableID.MatchString(hook.ID) {
			problems = append(problems, fmt.Errorf("lifecycle hook ID %q is invalid", hook.ID))
		}
		if _, exists := seenHooks[hook.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate lifecycle hook ID %q", hook.ID))
		}
		seenHooks[hook.ID] = struct{}{}
		if hook.Phase != "validate" && hook.Phase != "build" && hook.Phase != "package" {
			problems = append(problems, fmt.Errorf("lifecycle hook %q has an unsupported preparation phase", hook.ID))
		}
		if hook.Relationship != "before" && hook.Relationship != "after" && hook.Relationship != "replace" && hook.Relationship != "disable" {
			problems = append(problems, fmt.Errorf("lifecycle hook %q has an invalid relationship", hook.ID))
		}
		if !stableID.MatchString(hook.Target) {
			problems = append(problems, fmt.Errorf("lifecycle hook %q has an invalid target", hook.ID))
		}
		if hook.Relationship == "disable" && hook.Command != nil {
			problems = append(problems, fmt.Errorf("disabled lifecycle hook %q cannot define a command", hook.ID))
		}
		if hook.Relationship != "disable" && hook.Command == nil {
			problems = append(problems, fmt.Errorf("lifecycle hook %q requires a command", hook.ID))
		}
		if hook.Command != nil {
			if hook.Command.Executable != "composer" && hook.Command.Executable != "magento" {
				problems = append(problems, fmt.Errorf("lifecycle hook %q has an unsupported executable", hook.ID))
			}
			for _, argument := range hook.Command.Arguments {
				if argument == "" || strings.ContainsRune(argument, '\x00') {
					problems = append(problems, fmt.Errorf("lifecycle hook %q has an invalid argument", hook.ID))
				}
			}
		}
		for _, dependency := range hook.Dependencies {
			if !stableID.MatchString(dependency) {
				problems = append(problems, fmt.Errorf("lifecycle hook %q has an invalid dependency", hook.ID))
			}
		}
		timeout := hook.Timeout
		if timeout == 0 {
			timeout = 300
		}
		if timeout < 1 || timeout > 7200 {
			problems = append(problems, fmt.Errorf("lifecycle hook %q timeout must be between 1 and 7200 seconds", hook.ID))
		}
		attempts := hook.Retries.MaxAttempts
		if attempts == 0 {
			attempts = 1
		}
		if attempts < 1 || attempts > 5 {
			problems = append(problems, fmt.Errorf("lifecycle hook %q max attempts must be between 1 and 5", hook.ID))
		}
		if hook.Retries.DelaySeconds < 0 || hook.Retries.DelaySeconds > 3600 {
			problems = append(problems, fmt.Errorf("lifecycle hook %q retry delay must be between 0 and 3600 seconds", hook.ID))
		}
		if attempts > 1 && !hook.Retries.Idempotent {
			problems = append(problems, fmt.Errorf("lifecycle hook %q retries require idempotent=true", hook.ID))
		}
		if hook.Failure != "" && hook.Failure != "abort" && hook.Failure != "continue" {
			problems = append(problems, fmt.Errorf("lifecycle hook %q has an invalid failure action", hook.ID))
		}
	}
	return errors.Join(problems...)
}

func (request FinalizeRequest) validate() error {
	return errors.Join(
		validateRelativePath("prepared artifact", request.PreparedArtifact),
		validateRevision(request.SourceRevision),
		validateOCIDigest(request.ImageDigest),
	)
}

func (response Response) Validate() error {
	var problems []error
	if response.ProtocolVersion != ProtocolVersion {
		problems = append(problems, fmt.Errorf("unsupported build runner protocol version %d", response.ProtocolVersion))
	}
	switch response.Stage {
	case StagePrepare:
		if response.Prepare == nil || response.Finalize != nil {
			problems = append(problems, errors.New("prepare response requires only a prepare payload"))
		} else {
			problems = append(problems, response.Prepare.validate())
		}
	case StageFinalize:
		if response.Finalize == nil || response.Prepare != nil {
			problems = append(problems, errors.New("finalize response requires only a finalize payload"))
		} else {
			problems = append(problems, response.Finalize.validate())
		}
	default:
		problems = append(problems, fmt.Errorf("invalid build runner stage %q", response.Stage))
	}
	return errors.Join(problems...)
}

func (response PrepareResponse) validate() error {
	var problems []error
	problems = append(problems, validateRelativePath("prepared artifact", response.PreparedArtifact))
	if response.PHPVersion == "" {
		problems = append(problems, errors.New("prepared PHP version is required"))
	}
	problems = append(problems, validateUniqueStrings("PHP extensions", response.PHPExtensions, false))
	problems = append(problems, validateUniqueStrings("enabled modules", response.EnabledModules, false))
	problems = append(problems, validateUniqueStrings("runtime capabilities", response.RequiredRuntimeCapabilities, true))
	if len(response.Checksums) == 0 {
		problems = append(problems, errors.New("prepared artifact checksums are required"))
	}
	seen := make(map[string]struct{}, len(response.Checksums))
	for _, checksum := range response.Checksums {
		problems = append(problems, validateRelativePath("artifact checksum", checksum.Path))
		if !sha256Checksum.MatchString(checksum.SHA256) {
			problems = append(problems, fmt.Errorf("artifact %q has an invalid SHA-256 checksum", checksum.Path))
		}
		if _, exists := seen[checksum.Path]; exists {
			problems = append(problems, fmt.Errorf("duplicate artifact checksum path %q", checksum.Path))
		}
		seen[checksum.Path] = struct{}{}
	}
	return errors.Join(problems...)
}

func (response FinalizeResponse) validate() error {
	return errors.Join(
		validateOCIDigest(response.ImageDigest),
		validateRelativePath("manifest", response.ManifestPath),
		validateChecksum("manifest", response.ManifestSHA256),
	)
}

func validateRevision(revision string) error {
	if !sourceRevision.MatchString(revision) {
		return errors.New("sourceRevision must be a lowercase commit hash")
	}
	return nil
}

func validateOCIDigest(digest string) error {
	if !sha256Digest.MatchString(digest) {
		return errors.New("imageDigest must be a lowercase SHA-256 OCI digest")
	}
	return nil
}

func validateChecksum(name, checksum string) error {
	if !sha256Checksum.MatchString(checksum) {
		return fmt.Errorf("%s must be a lowercase SHA-256 checksum", name)
	}
	return nil
}

func validateRelativePath(name, path string) error {
	clean := pathpkg.Clean(path)
	if path == "" || strings.Contains(path, "\\") || pathpkg.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s path %q must be a portable relative path and cannot traverse its parent", name, path)
	}
	return nil
}

func validateUniqueStrings(name string, values []string, ids bool) error {
	if len(values) == 0 {
		return fmt.Errorf("%s cannot be empty", name)
	}
	seen := make(map[string]struct{}, len(values))
	var problems []error
	for _, value := range values {
		if strings.TrimSpace(value) == "" || ids && !stableID.MatchString(value) {
			problems = append(problems, fmt.Errorf("invalid %s value %q", name, value))
		}
		if _, exists := seen[value]; exists {
			problems = append(problems, fmt.Errorf("duplicate %s value %q", name, value))
		}
		seen[value] = struct{}{}
	}
	return errors.Join(problems...)
}

func canonicalRequest(request Request) Request {
	copy := request
	if request.Prepare != nil {
		prepare := *request.Prepare
		prepare.InputFiles = append(make([]InputFile, 0, len(prepare.InputFiles)), prepare.InputFiles...)
		sort.Slice(prepare.InputFiles, func(i, j int) bool { return prepare.InputFiles[i].Path < prepare.InputFiles[j].Path })
		prepare.StaticContent = append(make([]StaticContent, 0, len(prepare.StaticContent)), prepare.StaticContent...)
		sort.Slice(prepare.StaticContent, func(i, j int) bool {
			if prepare.StaticContent[i].Locale == prepare.StaticContent[j].Locale {
				return prepare.StaticContent[i].Theme < prepare.StaticContent[j].Theme
			}
			return prepare.StaticContent[i].Locale < prepare.StaticContent[j].Locale
		})
		prepare.LifecycleHooks = append(make([]LifecycleHook, 0, len(prepare.LifecycleHooks)), prepare.LifecycleHooks...)
		for i := range prepare.LifecycleHooks {
			hook := &prepare.LifecycleHooks[i]
			if hook.Timeout == 0 {
				hook.Timeout = 300
			}
			if hook.Retries.MaxAttempts == 0 {
				hook.Retries.MaxAttempts = 1
			}
			if hook.Failure == "" {
				hook.Failure = "abort"
			}
			hook.Dependencies = append([]string(nil), hook.Dependencies...)
			sort.Strings(hook.Dependencies)
			if hook.Command != nil {
				hook.Command.Arguments = append([]string(nil), hook.Command.Arguments...)
			}
		}
		sort.Slice(prepare.LifecycleHooks, func(i, j int) bool { return prepare.LifecycleHooks[i].ID < prepare.LifecycleHooks[j].ID })
		copy.Prepare = &prepare
	}
	return copy
}

func canonicalResponse(response Response) Response {
	copy := response
	if response.Prepare != nil {
		prepare := *response.Prepare
		prepare.PHPExtensions = sortedCopy(prepare.PHPExtensions)
		prepare.EnabledModules = sortedCopy(prepare.EnabledModules)
		prepare.RequiredRuntimeCapabilities = sortedCopy(prepare.RequiredRuntimeCapabilities)
		prepare.Checksums = append([]FileChecksum(nil), prepare.Checksums...)
		sort.Slice(prepare.Checksums, func(i, j int) bool { return prepare.Checksums[i].Path < prepare.Checksums[j].Path })
		copy.Prepare = &prepare
	}
	return copy
}

func sortedCopy(values []string) []string {
	copy := append([]string(nil), values...)
	sort.Strings(copy)
	return copy
}
