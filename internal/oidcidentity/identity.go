package oidcidentity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

const (
	EnvTokenFile                = "MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE"
	EnvToken                    = "MAGELIFT_COSIGN_IDENTITY_TOKEN"
	EnvTokenArgv                = "MAGELIFT_COSIGN_IDENTITY_TOKEN_ARGV"
	EnvSigningServiceAccount    = "MAGELIFT_SIGNING_SERVICE_ACCOUNT"
	envGCloudImpersonateAccount = "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"
)

// Identity is the Sigstore certificate subject and issuer extracted from an
// identity token. Callers must never log the token that produced it.
type Identity struct {
	Subject string
	Issuer  string
}

var (
	ErrTokenUnavailable = errors.New("OIDC identity token is unavailable")
	ErrInvalidToken     = errors.New("OIDC identity token is invalid")
	ErrInvalidSource    = errors.New("OIDC identity token source is invalid")
)

// Source yields a Sigstore OIDC identity token. Implementations must never
// return the token in an error. The Magento cloud provider is irrelevant:
// Google SA impersonation, GitHub Actions, and other OIDC issuers are all
// valid sources for Cosign keyless signing of any registry digest.
type Source interface {
	Token(context.Context) ([]byte, error)
}

type FileSource struct {
	Path string
}

type EnvSource struct {
	Value string
}

type CommandSource struct {
	Name   string
	Args   []string
	runner outputRunner
}

type outputRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type execOutputRunner struct{}

func (execOutputRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = nil
	command.Stderr = io.Discard
	return command.Output()
}

func Normalize(raw []byte) ([]byte, error) {
	token := bytes.TrimSpace(raw)
	if len(token) == 0 {
		return nil, ErrInvalidToken
	}
	if bytes.ContainsFunc(token, unicode.IsSpace) || bytes.ContainsFunc(token, unicode.IsControl) {
		return nil, ErrInvalidToken
	}
	if !bytes.Contains(token, []byte{'.'}) {
		return nil, ErrInvalidToken
	}
	out := make([]byte, len(token))
	copy(out, token)
	return out, nil
}

func (s FileSource) Token(context.Context) ([]byte, error) {
	if !validPath(s.Path) {
		return nil, ErrInvalidSource
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, ErrTokenUnavailable
	}
	token, err := Normalize(raw)
	clear(raw)
	return token, err
}

func (s EnvSource) Token(context.Context) ([]byte, error) {
	return Normalize([]byte(s.Value))
}

func (s CommandSource) Token(ctx context.Context) ([]byte, error) {
	if strings.TrimSpace(s.Name) == "" || strings.HasPrefix(s.Name, "-") || strings.IndexFunc(s.Name, unicode.IsControl) >= 0 {
		return nil, ErrInvalidSource
	}
	for _, arg := range s.Args {
		if strings.IndexFunc(arg, unicode.IsControl) >= 0 {
			return nil, ErrInvalidSource
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	runner := s.runner
	if runner == nil {
		runner = execOutputRunner{}
	}
	raw, err := runner.Output(ctx, s.Name, s.Args...)
	if err != nil {
		clear(raw)
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		return nil, ErrTokenUnavailable
	}
	token, err := Normalize(raw)
	clear(raw)
	return token, err
}

func Claims(token []byte) (Identity, error) {
	normalized, err := Normalize(token)
	if err != nil {
		return Identity{}, err
	}
	parts := bytes.Split(normalized, []byte{'.'})
	if len(parts) != 3 || len(parts[1]) == 0 {
		return Identity{}, ErrInvalidToken
	}
	payload, err := decodeJWTPayload(parts[1])
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	var claims struct {
		Iss   string `json:"iss"`
		Email string `json:"email"`
		Sub   string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, ErrInvalidToken
	}
	subject := strings.TrimSpace(claims.Email)
	if subject == "" {
		subject = strings.TrimSpace(claims.Sub)
	}
	issuer := strings.TrimSpace(claims.Iss)
	if subject == "" || strings.IndexFunc(subject, unicode.IsControl) >= 0 || !validIssuer(issuer) {
		return Identity{}, ErrInvalidToken
	}
	return Identity{Subject: subject, Issuer: issuer}, nil
}

func decodeJWTPayload(segment []byte) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(string(segment))
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(string(segment))
	}
	return decoded, err
}

func validIssuer(rawURL string) bool {
	issuer, err := url.Parse(rawURL)
	return err == nil && issuer.Scheme == "https" && issuer.Host != "" && issuer.User == nil && issuer.RawQuery == "" && issuer.Fragment == ""
}

func CloudCLISource(getenv func(string) string, lookPath func(string) (string, error), impersonate string) Source {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("gcloud"); err != nil {
		return nil
	}
	args := []string{"auth", "print-identity-token", "--audiences=sigstore", "--include-email"}
	if sa := signingServiceAccount(getenv, impersonate); sa != "" {
		args = append(args, "--impersonate-service-account", sa)
	}
	return CommandSource{Name: "gcloud", Args: args}
}

func SourceFromEnvOrCloudCLI(getenv func(string) string, lookPath func(string) (string, error), impersonate string) (Source, error) {
	source, err := SourceFromEnv(getenv)
	if err != nil || source != nil {
		return source, err
	}
	return CloudCLISource(getenv, lookPath, impersonate), nil
}

func signingServiceAccount(getenv func(string) string, impersonate string) string {
	if getenv != nil {
		if sa := strings.TrimSpace(getenv(EnvSigningServiceAccount)); sa != "" {
			return sa
		}
		if sa := strings.TrimSpace(getenv(envGCloudImpersonateAccount)); sa != "" {
			return sa
		}
	}
	return strings.TrimSpace(impersonate)
}

func SourceFromEnv(getenv func(string) string) (Source, error) {
	if getenv == nil {
		return nil, nil
	}
	file := strings.TrimSpace(getenv(EnvTokenFile))
	token := getenv(EnvToken)
	argv := strings.TrimSpace(getenv(EnvTokenArgv))
	set := 0
	if file != "" {
		set++
	}
	if strings.TrimSpace(token) != "" {
		set++
	}
	if argv != "" {
		set++
	}
	if set == 0 {
		return nil, nil
	}
	if set > 1 {
		return nil, ErrInvalidSource
	}
	switch {
	case file != "":
		return FileSource{Path: file}, nil
	case strings.TrimSpace(token) != "":
		return EnvSource{Value: token}, nil
	default:
		name, args, err := parseArgv(argv)
		if err != nil {
			return nil, err
		}
		return CommandSource{Name: name, Args: args}, nil
	}
}

func parseArgv(raw string) (string, []string, error) {
	var argv []string
	if err := json.Unmarshal([]byte(raw), &argv); err != nil || len(argv) == 0 {
		return "", nil, ErrInvalidSource
	}
	name := strings.TrimSpace(argv[0])
	if name == "" || strings.HasPrefix(name, "-") {
		return "", nil, ErrInvalidSource
	}
	args := append([]string(nil), argv[1:]...)
	return name, args, nil
}

func Materialize(ctx context.Context, source Source) (string, func(), error) {
	nop := func() {}
	if source == nil {
		return "", nop, nil
	}
	token, err := source.Token(ctx)
	if err != nil {
		return "", nop, err
	}
	defer clear(token)
	file, err := os.CreateTemp("", "magelift-oidc-*.jwt")
	if err != nil {
		return "", nop, ErrTokenUnavailable
	}
	path := file.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}
	if _, err := file.Write(token); err != nil {
		_ = file.Close()
		cleanup()
		return "", nop, ErrTokenUnavailable
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nop, ErrTokenUnavailable
	}
	return path, cleanup, nil
}

func validPath(path string) bool {
	return path != "" && !strings.HasPrefix(path, "-") && strings.IndexFunc(path, unicode.IsControl) < 0
}
