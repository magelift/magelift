package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const maxMetadataBytes = 4 << 20

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var revisionPattern = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
var secretIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
var buildArgName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var builderName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var forbiddenBuildArg = regexp.MustCompile(`(?i)(secret|token|password|credential|authorization|private.?key)`)
var forbiddenBuildArgValue = regexp.MustCompile(`(?i)^(?:aws-secrets-manager|ssm|gcp-secret-manager|vault)://|-----BEGIN PRIVATE KEY-----`)

type OutputMode string

const (
	OutputLoad OutputMode = "load"
	OutputPush OutputMode = "push"
)

type Source struct {
	LocalDirectory string
	GitURL         string
}

type Secret struct {
	ID    string
	Value []byte
}

type Request struct {
	Source           Source
	ProvenanceSource string
	Dockerfile       string
	ImageReference   string
	Platforms        []string
	Output           OutputMode
	BuildArgs        map[string]string
	Builder          string
	Secrets          []Secret
}

type Result struct {
	ImageReference string          `json:"imageReference" yaml:"imageReference"`
	Digest         string          `json:"digest" yaml:"digest"`
	Pushed         bool            `json:"pushed" yaml:"pushed"`
	Metadata       json.RawMessage `json:"-" yaml:"-"`
}

type commandFactory func(context.Context, string, ...string) *exec.Cmd

type Runner struct {
	diagnostics io.Writer
	command     commandFactory
}

func NewRunner(diagnostics io.Writer) *Runner {
	return &Runner{diagnostics: diagnostics, command: exec.CommandContext}
}

func (r *Runner) Build(ctx context.Context, request Request) (Result, error) {
	if err := validate(request, r.diagnostics); err != nil {
		return Result{}, err
	}
	temporaryDirectory, err := os.MkdirTemp("", "magelift-buildkit-")
	if err != nil {
		return Result{}, fmt.Errorf("create private BuildKit directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)
	metadataPath := filepath.Join(temporaryDirectory, "metadata.json")
	secretOptions, err := writeSecrets(temporaryDirectory, request.Secrets)
	if err != nil {
		return Result{}, err
	}

	args := buildArgs(request, metadataPath, secretOptions)
	cmd := r.command(ctx, "docker", args...)
	cmd.Stdout = r.diagnostics
	cmd.Stderr = r.diagnostics
	cmd.WaitDelay = 5 * time.Second
	if runtime.GOOS != "windows" {
		cmd.Cancel = func() error {
			err := cmd.Process.Signal(os.Interrupt)
			if errors.Is(err, os.ErrProcessDone) {
				return os.ErrProcessDone
			}
			return err
		}
	}
	if err := cmd.Run(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return Result{}, cause
		}
		return Result{}, fmt.Errorf("BuildKit build failed: %w", err)
	}
	digest, metadata, err := readMetadata(metadataPath)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ImageReference: request.ImageReference,
		Digest:         digest,
		Pushed:         request.Output == OutputPush,
		Metadata:       metadata,
	}, nil
}

func validate(request Request, diagnostics io.Writer) error {
	if diagnostics == nil {
		return errors.New("BuildKit diagnostics writer is required")
	}
	if (request.Source.LocalDirectory == "") == (request.Source.GitURL == "") {
		return errors.New("BuildKit request must select exactly one source")
	}
	if request.Dockerfile == "" || request.ImageReference == "" {
		return errors.New("BuildKit Dockerfile and image reference are required")
	}
	if request.Output != OutputLoad && request.Output != OutputPush {
		return errors.New("BuildKit output must be load or push")
	}
	if len(request.Platforms) == 0 {
		return errors.New("at least one target platform is required")
	}
	if request.Builder != "" && !builderName.MatchString(request.Builder) {
		return errors.New("BuildKit builder name is invalid")
	}
	if request.Output == OutputLoad && len(request.Platforms) != 1 {
		return errors.New("the local Docker exporter supports exactly one platform")
	}
	if request.Output == OutputPush {
		if err := validateReleaseSource(request); err != nil {
			return err
		}
		if !registryQualified(request.ImageReference) {
			return errors.New("pushed images require a registry-qualified reference")
		}
	}
	seenSecrets := map[string]bool{}
	for name, value := range request.BuildArgs {
		if !buildArgName.MatchString(name) || forbiddenBuildArg.MatchString(name) || forbiddenBuildArgValue.MatchString(value) {
			return fmt.Errorf("invalid or secret-bearing BuildKit build argument %q", name)
		}
	}
	for _, secret := range request.Secrets {
		if !secretIDPattern.MatchString(secret.ID) {
			return fmt.Errorf("invalid BuildKit secret ID %q", secret.ID)
		}
		if seenSecrets[secret.ID] {
			return fmt.Errorf("duplicate BuildKit secret ID %q", secret.ID)
		}
		if len(secret.Value) == 0 {
			return fmt.Errorf("BuildKit secret %q cannot be empty", secret.ID)
		}
		seenSecrets[secret.ID] = true
	}
	return nil
}

func validateReleaseSource(request Request) error {
	if request.Source.GitURL != "" {
		if err := validateImmutableHTTPSURL(request.Source.GitURL, "Git source", true); err != nil {
			return err
		}
		if request.ProvenanceSource == "" {
			return nil
		}
	}
	if request.ProvenanceSource == "" {
		return errors.New("pushed images built from a local directory require an immutable provenance source")
	}
	return validateImmutableHTTPSURL(request.ProvenanceSource, "provenance source", false)
}

func validateImmutableHTTPSURL(rawURL, label string, allowGitSelector bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("pushed images require a valid HTTPS %s", label)
	}
	query := parsed.Query()
	checksums := query["checksum"]
	for name, values := range query {
		allowed := name == "checksum" || allowGitSelector && (name == "ref" || name == "tag" || name == "branch")
		if !allowed || len(values) != 1 {
			return fmt.Errorf("pushed image %s contains unsupported query parameters", label)
		}
	}
	if len(checksums) != 1 || !revisionPattern.MatchString(checksums[0]) {
		return fmt.Errorf("pushed image %s requires a full commit checksum", label)
	}
	return nil
}

func registryQualified(reference string) bool {
	first, _, found := strings.Cut(reference, "/")
	return found && (first == "localhost" || strings.ContainsAny(first, ".:"))
}

func writeSecrets(directory string, secrets []Secret) ([]string, error) {
	options := make([]string, 0, len(secrets))
	for index, secret := range secrets {
		path := filepath.Join(directory, fmt.Sprintf("secret-%d", index))
		if err := os.WriteFile(path, secret.Value, 0o600); err != nil {
			return nil, fmt.Errorf("write private BuildKit secret: %w", err)
		}
		options = append(options, "type=file,id="+secret.ID+",src="+path)
	}
	return options, nil
}

func buildArgs(request Request, metadataPath string, secretOptions []string) []string {
	args := []string{
		"buildx", "build",
		"--file", request.Dockerfile,
		"--metadata-file", metadataPath,
		"--platform", strings.Join(request.Platforms, ","),
		"--progress", "plain",
		"--tag", request.ImageReference,
	}
	if request.Builder != "" {
		args = append(args, "--builder", request.Builder)
	}
	if request.Output == OutputLoad {
		args = append(args, "--load")
	} else {
		args = append(args, "--sbom=true", "--provenance=mode=max,version=v1", "--push")
	}
	for _, name := range sortedKeys(request.BuildArgs) {
		args = append(args, "--build-arg", name+"="+request.BuildArgs[name])
	}
	for _, option := range secretOptions {
		args = append(args, "--secret", option)
	}
	source := request.Source.GitURL
	if source == "" {
		source = filepath.Clean(request.Source.LocalDirectory)
	}
	return append(args, source)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func readMetadata(path string) (string, json.RawMessage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("open BuildKit metadata: %w", err)
	}
	if info.Size() > maxMetadataBytes {
		return "", nil, errors.New("BuildKit metadata exceeds the size limit")
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("BuildKit metadata must be a regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read BuildKit metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var metadata struct {
		Digest string `json:"containerimage.digest"`
	}
	if err := decoder.Decode(&metadata); err != nil {
		return "", nil, fmt.Errorf("decode BuildKit metadata: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return "", nil, err
	}
	if !digestPattern.MatchString(metadata.Digest) {
		return "", nil, errors.New("BuildKit metadata does not contain a valid immutable image digest")
	}
	return metadata.Digest, json.RawMessage(contents), nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("BuildKit metadata contains trailing data")
	}
	return nil
}
