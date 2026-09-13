package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/oidcidentity"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/releasejournal"
)

const releaseOne = "registry.example.invalid/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const releaseTwo = "registry.example.invalid/shop@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestPromoteRollbackAndHistoryUseForwardJournalEntries(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	verified := 0
	newOptions := func(stdout, stderr *bytes.Buffer) *options {
		o := testOptions(stdout, &fakeTerminal{interactive: false})
		o.stderr = stderr
		o.configPath = configPath
		o.environment = "staging"
		o.output = "json"
		o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
		o.verifyRelease = func(_ context.Context, reference string, options cosign.VerifyOptions) error {
			verified++
			if reference == "" || options.CertificateIdentity == "" || options.OIDCIssuer == "" {
				t.Fatal("signature policy was incomplete")
			}
			return nil
		}
		o.liveSchemaEpoch = func(context.Context) (int, error) { return 1, nil }
		return o
	}
	for _, digest := range []string{releaseOne, releaseTwo} {
		var stdout, stderr bytes.Buffer
		command := newCommandWithOptions(newOptions(&stdout, &stderr))
		command.SetArgs([]string{"promote", "--digest", digest, "--certificate-identity", "release@example.invalid", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		if stderr.Len() != 0 || !strings.Contains(stdout.String(), `"forwardOnly": true`) {
			t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
		}
	}
	if verified != 2 {
		t.Fatalf("signature verifications = %d", verified)
	}

	var rollbackOut, rollbackErr bytes.Buffer
	rollback := newCommandWithOptions(newOptions(&rollbackOut, &rollbackErr))
	rollback.SetArgs([]string{"rollback", "--to-sequence", "1", "--ack-forward-only"})
	if err := rollback.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rollbackErr.String(), "Database migrations are not reversed") || !strings.Contains(rollbackErr.String(), "expand/contract") || !strings.Contains(rollbackOut.String(), releaseOne) || !strings.Contains(rollbackOut.String(), `"sourceSequence": 1`) || !strings.Contains(rollbackOut.String(), `"databaseMigrationsReversed": false`) {
		t.Fatalf("stdout=%s stderr=%s", rollbackOut.String(), rollbackErr.String())
	}

	var historyOut, historyErr bytes.Buffer
	history := newCommandWithOptions(newOptions(&historyOut, &historyErr))
	history.SetArgs([]string{"history"})
	if err := history.Execute(); err != nil {
		t.Fatal(err)
	}
	if historyErr.Len() != 0 || strings.Count(historyOut.String(), `"sequence"`) != 3 || !strings.Contains(historyOut.String(), `"action": "rollback"`) {
		t.Fatalf("stdout=%s stderr=%s", historyOut.String(), historyErr.String())
	}
}

func TestRollbackRefusesOlderSchemaBeforeCutover(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	epoch := 1
	newOptions := func(stdout, stderr *bytes.Buffer) *options {
		o := testOptions(stdout, &fakeTerminal{interactive: false})
		o.stderr = stderr
		o.configPath = configPath
		o.environment = "staging"
		o.output = "json"
		o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
		o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error { return nil }
		o.liveSchemaEpoch = func(context.Context) (int, error) { return epoch, nil }
		return o
	}
	for _, digest := range []string{releaseOne, releaseTwo} {
		var stdout, stderr bytes.Buffer
		command := newCommandWithOptions(newOptions(&stdout, &stderr))
		command.SetArgs([]string{"promote", "--digest", digest, "--certificate-identity", "release@example.invalid", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		epoch++
	}
	deployed := false
	var stdout, stderr bytes.Buffer
	o := newOptions(&stdout, &stderr)
	o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
		deployed = true
		return fakeDeploymentSteps{}, nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"rollback", "--to-sequence", "1", "--ack-forward-only"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(err.Error(), "cannot run on the live schema") || !strings.Contains(err.Error(), "restore the previous database") || !strings.Contains(err.Error(), "forward-fix") {
		t.Fatalf("error = %v", err)
	}
	if deployed {
		t.Fatal("rollback cut over after a schema mismatch")
	}
}

func TestRollbackRefusesWhenSchemaEpochIsMissing(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath = configPath
	o.environment = "staging"
	o.output = "json"
	o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error { return nil }
	for _, digest := range []string{releaseOne, releaseTwo} {
		command := newCommandWithOptions(o)
		command.SetArgs([]string{"promote", "--digest", digest, "--certificate-identity", "release@example.invalid", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"rollback", "--to-sequence", "1", "--ack-forward-only"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "live schema compatibility is unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestRollbackRefusesUnsignedJournalWithoutPromoteMetadata(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath = configPath
	o.environment = "staging"
	o.output = "json"
	o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error {
		t.Fatal("unsigned journal reached Cosign verification")
		return nil
	}
	store, err := releasejournal.New(directory, "staging")
	if err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{releaseOne, releaseTwo} {
		if _, err := store.Append(context.Background(), releasejournal.Entry{
			Action: releasejournal.ActionDeploy, Environment: "staging", DigestReference: digest, ForwardOnly: true, SchemaEpoch: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"rollback", "--to-sequence", "1", "--ack-forward-only"})
	err = command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "no verified signature metadata") || !strings.Contains(err.Error(), "promote the digest") {
		t.Fatalf("error = %v", err)
	}
}

func TestRollbackUsesPromoteSignaturesWhenDeployRowIsUnsigned(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	epoch := 1
	newOptions := func(stdout, stderr *bytes.Buffer) *options {
		o := testOptions(stdout, &fakeTerminal{interactive: false})
		o.stderr = stderr
		o.configPath = configPath
		o.environment = "staging"
		o.output = "json"
		o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
		o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error { return nil }
		o.liveSchemaEpoch = func(context.Context) (int, error) { return epoch, nil }
		return o
	}
	for _, digest := range []string{releaseOne, releaseTwo} {
		var stdout, stderr bytes.Buffer
		command := newCommandWithOptions(newOptions(&stdout, &stderr))
		command.SetArgs([]string{"promote", "--digest", digest, "--certificate-identity", "release@example.invalid", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		store, err := releasejournal.New(directory, "staging")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Append(context.Background(), releasejournal.Entry{
			Action: releasejournal.ActionDeploy, Environment: "staging", DigestReference: digest, ForwardOnly: true,
		}); err != nil {
			t.Fatal(err)
		}
		epoch++
	}
	deployed := false
	var stdout, stderr bytes.Buffer
	o := newOptions(&stdout, &stderr)
	o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
		deployed = true
		return fakeDeploymentSteps{}, nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"rollback", "--to-sequence", "2", "--ack-forward-only"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(err.Error(), "cannot run on the live schema") {
		t.Fatalf("error = %v", err)
	}
	if deployed {
		t.Fatal("rollback cut over after a schema mismatch")
	}
}

func TestRollbackRequiresForwardOnlyAcknowledgement(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath = configPath
	o.environment = "staging"
	o.output = "json"
	o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"rollback", "--to-sequence", "1"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--ack-forward-only") {
		t.Fatalf("error = %v", err)
	}
}

func TestProductionReleaseChangesRequireApproval(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	production := strings.Replace(starterConfig, "    account: \"123456789012\"", "    account: \"123456789012\"\n    class: production", 1)
	if err := os.WriteFile(configPath, []byte(production), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment = configPath, "staging"
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error {
		t.Fatal("verification ran before approval")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"promote", "--digest", releaseOne, "--certificate-identity", "identity", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error = %v", err)
	}
}

func TestPromoteRejectsUnsignedOrMutableReferences(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment = configPath, "staging"
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error {
		t.Fatal("mutable reference reached verification")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"promote", "--digest", "registry.example.invalid/shop:latest", "--certificate-identity", "identity", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
	if err := command.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v", err)
	}
}

func TestEvidenceExportsDigestActorProvenanceWithoutSecrets(t *testing.T) {
	path := writeLifecycleConfig(t, "production", true)
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newReleaseStore = func(string, string) (releaseStore, error) {
		return fakeReleaseStore{entries: []releasejournal.Entry{{
			Sequence:          3,
			Action:            releasejournal.ActionDeploy,
			DigestReference:   releaseOne,
			SignatureIdentity: "release@example.invalid",
			SchemaEpoch:       12,
		}}}, nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "evidence"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "database-password") || strings.Contains(got, "super-secret") {
		t.Fatalf("secret leaked: %s", got)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, got)
	}
	if result["environment"] != "staging" {
		t.Fatalf("environment = %#v", result["environment"])
	}
	if result["configDigest"] == "" || result["configDigest"] == nil {
		t.Fatalf("missing configDigest: %#v", result)
	}
	if _, ok := result["configProvenance"].(map[string]any); !ok {
		t.Fatalf("configProvenance = %#v", result["configProvenance"])
	}
	if _, ok := result["backupPolicy"].(map[string]any); !ok {
		t.Fatalf("backupPolicy = %#v", result["backupPolicy"])
	}
	changes, ok := result["changes"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("changes = %#v", result["changes"])
	}
	change, _ := changes[0].(map[string]any)
	if change["digest"] != releaseOne || change["actor"] != "release@example.invalid" {
		t.Fatalf("change = %#v", change)
	}
}

func TestPromoteDiscoversIdentityFromLoginToken(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	token := signIdentityJWT(t, "release@example.invalid", "https://issuer.example.invalid")
	var verified cosign.VerifyOptions
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	o.output = "json"
	o.getenv = func(key string) string {
		if key == oidcidentity.EnvToken {
			return token
		}
		return ""
	}
	o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
	o.verifyRelease = func(_ context.Context, reference string, options cosign.VerifyOptions) error {
		if reference != releaseOne {
			t.Fatalf("digest = %q", reference)
		}
		verified = options
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"promote", "--digest", releaseOne})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if verified.CertificateIdentity != "release@example.invalid" || verified.OIDCIssuer != "https://issuer.example.invalid" {
		t.Fatalf("verified = %#v", verified)
	}
}

func TestPromoteRefusesWhenLoginIdentityCannotBeDiscovered(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error {
		t.Fatal("verifyRelease was called")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"promote", "--digest", releaseOne})
	if err := command.Execute(); err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "cannot verify the signed image") {
		t.Fatalf("error = %v", err)
	}
}
