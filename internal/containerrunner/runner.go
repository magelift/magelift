package containerrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultMaxStdout int64 = 1 << 20

var pinnedImage = regexp.MustCompile(`^(?:sha256:[a-f0-9]{64}|[^\s@]+@sha256:[a-f0-9]{64})$`)
var secretID = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

var ErrStdoutLimit = errors.New("container runner stdout limit exceeded")

type Runner struct {
	Binary    string
	Image     string
	TempRoot  string
	Stderr    io.Writer
	MaxStdout int64
	Network   string
}

type Secret struct {
	ID    string
	Value []byte
}

type Result struct {
	Response  []byte
	OutputDir string
}

func (result Result) Cleanup() error {
	if result.OutputDir == "" {
		return nil
	}
	return os.RemoveAll(result.OutputDir)
}

func (runner Runner) Run(ctx context.Context, sourceDir string, protocolJSON []byte, secrets ...Secret) (Result, error) {
	outputDir, err := os.MkdirTemp(runner.TempRoot, "magelift-build-*")
	if err != nil {
		return Result{}, fmt.Errorf("create private build output: %w", err)
	}
	if err := os.Chmod(outputDir, 0o700); err != nil {
		_ = os.RemoveAll(outputDir)
		return Result{}, fmt.Errorf("protect build output: %w", err)
	}
	result, err := runner.run(ctx, sourceDir, outputDir, protocolJSON, secrets...)
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return Result{}, err
	}
	return result, nil
}

// RunInOutput reuses the private directory returned by Run so finalize can
// read prepared metadata after BuildKit has produced the image digest.
func (runner Runner) RunInOutput(ctx context.Context, sourceDir, outputDir string, protocolJSON []byte, secrets ...Secret) (Result, error) {
	return runner.run(ctx, sourceDir, outputDir, protocolJSON, secrets...)
}

func (runner Runner) run(ctx context.Context, sourceDir, outputDir string, protocolJSON []byte, secrets ...Secret) (Result, error) {
	if err := runner.validate(protocolJSON, secrets); err != nil {
		return Result{}, err
	}
	sourceDir, err := cleanMountDirectory(sourceDir, false)
	if err != nil {
		return Result{}, err
	}
	outputDir, err = cleanMountDirectory(outputDir, true)
	if err != nil {
		return Result{}, err
	}
	if mountPathsOverlap(sourceDir, outputDir) {
		return Result{}, errors.New("container runner output must not overlap the read-only source directory")
	}

	result := Result{OutputDir: outputDir}

	secretMounts, cleanupSecrets, err := materializeSecrets(secrets)
	if err != nil {
		return Result{}, err
	}
	defer cleanupSecrets()

	arguments := []string{
		"run", "--rm", "-i",
		"--user=" + containerUser(),
		"--network=" + runner.network(),
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,mode=1777",
		"--env", "HOME=/tmp",
		"--mount", mountArgument(sourceDir, "/workspace", true),
		"--mount", mountArgument(outputDir, "/output", false),
		"--workdir", "/workspace",
	}
	for _, mount := range secretMounts {
		arguments = append(arguments, "--mount", mount)
	}
	arguments = append(arguments, runner.Image)
	command := exec.CommandContext(ctx, runner.binary(), arguments...)
	command.Env = []string{}
	command.Stdin = bytes.NewReader(protocolJSON)
	command.Stderr = runner.stderr()
	stdout := &limitedBuffer{maximum: runner.maximumStdout()}
	command.Stdout = stdout

	err = command.Run()
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("container runner canceled: %w", ctx.Err())
	}
	if errors.Is(stdout.err, ErrStdoutLimit) {
		return Result{}, ErrStdoutLimit
	}
	if err != nil {
		return Result{}, fmt.Errorf("run isolated build container: %w", err)
	}
	result.Response = append([]byte(nil), stdout.buffer.Bytes()...)
	return result, nil
}

func (runner Runner) validate(protocolJSON []byte, secrets []Secret) error {
	if !pinnedImage.MatchString(runner.Image) {
		return errors.New("builder image must be pinned by a lowercase SHA-256 digest")
	}
	if !json.Valid(protocolJSON) {
		return errors.New("build runner input must be valid JSON")
	}
	if runner.maximumStdout() < 1 {
		return errors.New("container runner stdout limit must be positive")
	}
	if runner.Network != "" && runner.Network != "none" && runner.Network != "bridge" {
		return errors.New("container runner network must be none or bridge")
	}
	seen := map[string]bool{}
	for _, secret := range secrets {
		if !secretID.MatchString(secret.ID) || len(secret.Value) == 0 {
			return errors.New("container runner secrets require a stable ID and non-empty value")
		}
		if seen[secret.ID] {
			return fmt.Errorf("duplicate container runner secret ID %q", secret.ID)
		}
		seen[secret.ID] = true
	}
	return nil
}

func (runner Runner) network() string {
	if runner.Network == "" {
		return "none"
	}
	return runner.Network
}

func materializeSecrets(secrets []Secret) ([]string, func(), error) {
	directory, err := os.MkdirTemp("", "magelift-runner-secrets-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create private runner secret directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	mounts := make([]string, 0, len(secrets))
	for index, secret := range secrets {
		path := filepath.Join(directory, fmt.Sprintf("secret-%d", index))
		if err := os.WriteFile(path, secret.Value, 0o600); err != nil {
			cleanup()
			return nil, func() {}, errors.New("write private runner secret")
		}
		mounts = append(mounts, "type=bind,src="+path+",dst=/run/secrets/"+secret.ID+",readonly")
	}
	return mounts, cleanup, nil
}

func (runner Runner) binary() string {
	if runner.Binary == "" {
		return "docker"
	}
	return runner.Binary
}

func (runner Runner) stderr() io.Writer {
	if runner.Stderr == nil {
		return io.Discard
	}
	return runner.Stderr
}

func (runner Runner) maximumStdout() int64 {
	if runner.MaxStdout == 0 {
		return DefaultMaxStdout
	}
	return runner.MaxStdout
}

func cleanMountDirectory(directory string, private bool) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve mount directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve mount directory links: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect mount directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("container runner mount source must be a directory")
	}
	if private && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("container runner output must not be accessible by group or other users")
	}
	if strings.ContainsAny(resolved, ",\r\n") {
		return "", errors.New("container runner mount path contains characters unsupported by Docker mounts")
	}
	return resolved, nil
}

func mountArgument(source, destination string, readOnly bool) string {
	argument := "type=bind,src=" + source + ",dst=" + destination
	if readOnly {
		argument += ",readonly"
	}
	return argument
}

func mountPathsOverlap(first, second string) bool {
	return pathContains(first, second) || pathContains(second, first)
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type limitedBuffer struct {
	buffer  bytes.Buffer
	maximum int64
	err     error
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.maximum - int64(buffer.buffer.Len())
	if remaining <= 0 {
		buffer.err = ErrStdoutLimit
		return len(data), nil
	}
	if int64(len(data)) > remaining {
		_, _ = buffer.buffer.Write(data[:remaining])
		buffer.err = ErrStdoutLimit
		return len(data), nil
	}
	return buffer.buffer.Write(data)
}
