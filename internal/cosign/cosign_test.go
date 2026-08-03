package cosign

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const testReference = "registry.example.invalid/team/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	mu       sync.Mutex
	commands []recordedCommand
	err      error
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	return r.err
}

func (r *recordingRunner) command(t *testing.T) recordedCommand {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.commands) != 1 {
		t.Fatalf("commands = %#v", r.commands)
	}
	return r.commands[0]
}

func TestSignUsesDigestOnlyArgv(t *testing.T) {
	runner := &recordingRunner{}
	client := NewWithRunner(runner)
	if err := client.Sign(context.Background(), testReference); err != nil {
		t.Fatal(err)
	}
	command := runner.command(t)
	if command.name != "cosign" || !reflect.DeepEqual(command.args, []string{"sign", "--yes", testReference}) {
		t.Fatalf("command = %#v", command)
	}
}

func TestVerifyUsesExactIdentityPolicyArgv(t *testing.T) {
	runner := &recordingRunner{}
	client := NewWithRunner(runner)
	options := VerifyOptions{
		CertificateIdentity: "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/heads/main",
		OIDCIssuer:          "https://token.actions.githubusercontent.com",
	}
	if err := client.Verify(context.Background(), testReference, options); err != nil {
		t.Fatal(err)
	}
	command := runner.command(t)
	want := []string{"verify", "--certificate-identity", options.CertificateIdentity, "--certificate-oidc-issuer", options.OIDCIssuer, testReference}
	if command.name != "cosign" || !reflect.DeepEqual(command.args, want) {
		t.Fatalf("command = %#v", command)
	}
}

func TestVerifyBlobUsesExpectedOptions(t *testing.T) {
	runner := &recordingRunner{}
	client := NewWithRunner(runner)
	options := VerifyOptions{
		CertificateIdentity: "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v1.2.3",
		OIDCIssuer:          "https://token.actions.githubusercontent.com",
	}
	if err := client.VerifyBlob(context.Background(), "/tmp/checksums.sigstore.json", "/tmp/checksums.txt", options); err != nil {
		t.Fatal(err)
	}
	command := runner.command(t)
	want := []string{"verify-blob", "--bundle", "/tmp/checksums.sigstore.json", "--certificate-identity", options.CertificateIdentity, "--certificate-oidc-issuer", options.OIDCIssuer, "/tmp/checksums.txt"}
	if command.name != "cosign" || !reflect.DeepEqual(command.args, want) {
		t.Fatalf("command = %#v", command)
	}
}

func TestRejectsNonDigestAndUnqualifiedReferences(t *testing.T) {
	tests := []string{
		"registry.example.invalid/team/shop:latest",
		"team/shop@sha256:" + strings.Repeat("a", 64),
		"https://registry.example.invalid/team/shop@sha256:" + strings.Repeat("a", 64),
		"registry.example.invalid/Team/shop@sha256:" + strings.Repeat("a", 64),
		"user@registry.example.invalid/team/shop@sha256:" + strings.Repeat("a", 64),
		"registry.example.invalid:99999/team/shop@sha256:" + strings.Repeat("a", 64),
		"registry.example.invalid/team/shop@sha256:short",
	}
	for _, reference := range tests {
		t.Run(reference, func(t *testing.T) {
			runner := &recordingRunner{}
			if err := NewWithRunner(runner).Sign(context.Background(), reference); !errors.Is(err, ErrInvalidReference) {
				t.Fatalf("error = %v", err)
			}
			if len(runner.commands) != 0 {
				t.Fatal("cosign was executed")
			}
		})
	}
}

func TestVerifyRejectsUnsafePolicy(t *testing.T) {
	tests := []VerifyOptions{
		{OIDCIssuer: "https://token.actions.githubusercontent.com"},
		{CertificateIdentity: "identity\nvalue", OIDCIssuer: "https://token.actions.githubusercontent.com"},
		{CertificateIdentity: "identity", OIDCIssuer: "http://token.actions.githubusercontent.com"},
		{CertificateIdentity: "identity", OIDCIssuer: "https://user@token.actions.githubusercontent.com"},
		{CertificateIdentity: "identity", OIDCIssuer: "https://token.actions.githubusercontent.com?tenant=one"},
	}
	for _, options := range tests {
		runner := &recordingRunner{}
		if err := NewWithRunner(runner).Verify(context.Background(), testReference, options); err == nil {
			t.Fatalf("options accepted: %#v", options)
		}
		if len(runner.commands) != 0 {
			t.Fatal("cosign was executed")
		}
	}
}

func TestCommandFailuresAreRedacted(t *testing.T) {
	secretOutput := "registry response contains sensitive-token"
	runner := &recordingRunner{err: errors.New(secretOutput)}
	client := NewWithRunner(runner)
	if err := client.Sign(context.Background(), testReference); !errors.Is(err, ErrSignFailed) || strings.Contains(err.Error(), secretOutput) {
		t.Fatalf("sign error = %v", err)
	}

	runner = &recordingRunner{err: errors.New(secretOutput)}
	client = NewWithRunner(runner)
	err := client.Verify(context.Background(), testReference, VerifyOptions{CertificateIdentity: "identity", OIDCIssuer: "https://issuer.example.invalid"})
	if !errors.Is(err, ErrVerifyFailed) || strings.Contains(err.Error(), secretOutput) {
		t.Fatalf("verify error = %v", err)
	}
}

func TestContextCancellationIsPreserved(t *testing.T) {
	cause := errors.New("release canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	runner := &recordingRunner{}
	err := NewWithRunner(runner).Sign(ctx, testReference)
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatal("cosign was executed after cancellation")
	}
}

func TestNilRunnerIsRejected(t *testing.T) {
	err := NewWithRunner(nil).Sign(context.Background(), testReference)
	if !errors.Is(err, ErrRunnerRequired) {
		t.Fatalf("error = %v", err)
	}
}
