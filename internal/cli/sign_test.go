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
