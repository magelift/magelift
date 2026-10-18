package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/oidcidentity"
)

const signJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.signature"

func TestSignUsesIdentityTokenFileWithoutPuttingJWTOnOptionsPath(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.jwt")
	if err := os.WriteFile(tokenPath, []byte(signJWT+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var signed cosign.SignOptions
	var reference string
	stdout := &bytes.Buffer{}
	o := testOptions(stdout, &fakeTerminal{})
	o.signRelease = func(_ context.Context, digest string, options cosign.SignOptions) error {
		reference = digest
		signed = options
		raw, err := os.ReadFile(options.IdentityTokenPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != signJWT {
			t.Fatalf("materialized token = %q", raw)
		}
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne, "--identity-token-file", tokenPath})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if reference != releaseOne {
		t.Fatalf("digest = %q", reference)
	}
	if signed.IdentityTokenPath == "" || signed.IdentityTokenPath == tokenPath || signed.IdentityTokenPath == signJWT {
		t.Fatalf("identity token path = %q", signed.IdentityTokenPath)
	}
	if strings.Contains(signed.IdentityTokenPath, signJWT) {
		t.Fatal("JWT leaked into the Cosign path")
	}
}

func TestSignUsesAmbientOIDCWhenNoTokenSource(t *testing.T) {
	var signed cosign.SignOptions
	stdout := &bytes.Buffer{}
	o := testOptions(stdout, &fakeTerminal{})
	o.signRelease = func(_ context.Context, _ string, options cosign.SignOptions) error {
		signed = options
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if signed.IdentityTokenPath != "" {
		t.Fatalf("identity token path = %q", signed.IdentityTokenPath)
	}
}

func TestSignRejectsConflictingTokenSources(t *testing.T) {
	stdout := &bytes.Buffer{}
	o := testOptions(stdout, &fakeTerminal{})
	o.getenv = func(key string) string {
		switch key {
		case oidcidentity.EnvTokenFile:
			return "/tmp/token.jwt"
		case oidcidentity.EnvTokenArgv:
			return `["gcloud","auth","print-identity-token"]`
		default:
			return ""
		}
	}
	o.signRelease = func(context.Context, string, cosign.SignOptions) error {
		t.Fatal("signRelease was called")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne})
	if err := command.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v", err)
	}
}

func TestSignRejectsNonDigest(t *testing.T) {
	stdout := &bytes.Buffer{}
	o := testOptions(stdout, &fakeTerminal{})
	o.signRelease = func(context.Context, string, cosign.SignOptions) error {
		t.Fatal("signRelease was called")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", "registry.example.invalid/shop:latest"})
	if err := command.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v", err)
	}
}

func TestGCPSigningServiceAccountMatchesBootstrapCIAccount(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(gcpStarterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	got := o.gcpSigningServiceAccount()
	want := "ml-example-shop-staging-ci@example-gcp-project.iam.gserviceaccount.com"
	if got != want {
		t.Fatalf("service account = %q, want %q", got, want)
	}
}

func TestSignUsesAmbientGitHubOIDCDespiteDiscoverableGcloud(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(gcpStarterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var signed cosign.SignOptions
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	o.getenv = func(key string) string {
		switch key {
		case envGitHubActions:
			return "true"
		case envGitHubIDTokenRequestURL:
			return "https://token.actions.githubusercontent.com"
		case envGitHubIDTokenRequestToken:
			return "oidc-request-token"
		default:
			return ""
		}
	}
	o.lookPath = func(name string) (string, error) {
		if name == "gcloud" {
			t.Fatal("gcloud must not be resolved when GitHub Actions OIDC is ambient")
		}
		return "", os.ErrNotExist
	}
	o.signRelease = func(_ context.Context, _ string, options cosign.SignOptions) error {
		signed = options
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if signed.IdentityTokenPath != "" {
		t.Fatalf("identity token path = %q", signed.IdentityTokenPath)
	}
}

func TestSignExplicitTokenOverridesGitHubActionsAmbient(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.jwt")
	if err := os.WriteFile(tokenPath, []byte(signJWT+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var signed cosign.SignOptions
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{})
	o.getenv = func(key string) string {
		switch key {
		case envGitHubActions:
			return "true"
		case envGitHubIDTokenRequestURL:
			return "https://token.actions.githubusercontent.com"
		case envGitHubIDTokenRequestToken:
			return "oidc-request-token"
		default:
			return ""
		}
	}
	o.signRelease = func(_ context.Context, _ string, options cosign.SignOptions) error {
		signed = options
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne, "--identity-token-file", tokenPath})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if signed.IdentityTokenPath == "" {
		t.Fatal("expected materialized identity token path")
	}
}

func TestSignGitHubActionsRejectsIncompleteOIDCRequestEnv(t *testing.T) {
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{})
	o.getenv = func(key string) string {
		switch key {
		case envGitHubActions:
			return "true"
		case envGitHubIDTokenRequestURL:
			return "https://token.actions.githubusercontent.com"
		default:
			return ""
		}
	}
	o.signRelease = func(context.Context, string, cosign.SignOptions) error {
		t.Fatal("signRelease was called")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "id-token: write") {
		t.Fatalf("error = %v", err)
	}
}

func TestSignGitHubActionsRejectsMissingOIDCPermission(t *testing.T) {
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{})
	o.getenv = func(key string) string {
		if key == envGitHubActions {
			return "true"
		}
		return ""
	}
	o.signRelease = func(context.Context, string, cosign.SignOptions) error {
		t.Fatal("signRelease was called")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"sign", "--digest", releaseOne})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "id-token: write") {
		t.Fatalf("error = %v", err)
	}
}

func TestSignUsesGcloudImpersonationFromYAMLWithoutExecuting(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(gcpStarterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	o.lookPath = func(name string) (string, error) {
		if name == "gcloud" {
			return "/usr/bin/gcloud", nil
		}
		return "", os.ErrNotExist
	}
	source, explicit, err := o.identitySource("")
	if err != nil || explicit {
		t.Fatalf("explicit=%v err=%v", explicit, err)
	}
	command, ok := source.(oidcidentity.CommandSource)
	if !ok || command.Name != "gcloud" || !strings.Contains(strings.Join(command.Args, " "), "--impersonate-service-account ml-example-shop-staging-ci@example-gcp-project.iam.gserviceaccount.com") {
		t.Fatalf("source = %#v", source)
	}
}

const gcpStarterConfig = `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
build:
  php: "8.5"
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    region: europe-west1
defaults:
  region: europe-west1
  preset: preview
environments:
  staging:
    account: "example-gcp-project"
extensions: {}
`

func signIdentityJWT(t *testing.T, email, issuer string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"email": email, "iss": issuer, "sub": email})
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestSignDetectsAmbientGitHubOIDCWithoutRunnerMarker(t *testing.T) {
	o := &options{
		getenv: func(name string) string {
			switch name {
			case envGitHubIDTokenRequestURL:
				return "https://example.invalid/oidc?api-version=2"
			case envGitHubIDTokenRequestToken:
				return "synthetic-request-credential"
			default:
				return ""
			}
		},
		lookPath: func(name string) (string, error) { t.Fatalf("ambient OIDC must not probe %s", name); return "", nil },
	}
	opts, cleanup, err := o.identitySignOptions(context.Background(), "")
	defer cleanup()
	if err != nil || opts.IdentityTokenPath != "" {
		t.Fatalf("options=%+v error=%v", opts, err)
	}
}
