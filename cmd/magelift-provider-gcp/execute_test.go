package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/magelift/magelift/internal/providerhost"
)

type recordingOutputsSource struct {
	decrypted map[string]any
	redacted  map[string]any
	calls     []string
}

func (s *recordingOutputsSource) Outputs(context.Context) (map[string]any, error) {
	s.calls = append(s.calls, "decrypted")
	return s.decrypted, nil
}

func (s *recordingOutputsSource) RedactedOutputs(context.Context) (map[string]any, error) {
	s.calls = append(s.calls, "redacted")
	return s.redacted, nil
}

// TestExecuteOutputsRoutesDecryptedToDay2 pins the day-2 contract: the
// outputs op serves secrets decrypted (exec needs the real kubeconfig),
// while only the redacted-outputs op redacts for display. Serving redacted
// from the outputs op fails day-2 with `Pulumi output "kubeconfig" must be
// a non-empty string`.
func TestExecuteOutputsRoutesDecryptedToDay2(t *testing.T) {
	source := &recordingOutputsSource{
		decrypted: map[string]any{"kubeconfig": "apiVersion: v1\nkind: Config\n"},
		redacted:  map[string]any{"kubeconfig": map[string]any{"secret": true}},
	}
	outputs, err := executeOutputs(context.Background(), source, providerhost.ExecuteOutputs)
	if err != nil {
		t.Fatal(err)
	}
	if outputs["kubeconfig"] != "apiVersion: v1\nkind: Config\n" {
		t.Fatalf("outputs = %v, want decrypted kubeconfig", outputs)
	}
	display, err := executeOutputs(context.Background(), source, providerhost.ExecuteRedactedOutputs)
	if err != nil {
		t.Fatal(err)
	}
	marker, ok := display["kubeconfig"].(map[string]any)
	if !ok || marker["secret"] != true {
		t.Fatalf("redacted = %v, want secret marker", display)
	}
	if len(source.calls) != 2 || source.calls[0] != "decrypted" || source.calls[1] != "redacted" {
		t.Fatalf("calls = %v", source.calls)
	}
}

func TestClassifyExecuteErrorFlagsConcurrentUpdate(t *testing.T) {
	var diagnostics bytes.Buffer
	diagnostics.WriteString("stderr: the stack is currently locked by ci\n")
	result, err := classifyExecuteError(
		providerhost.ExecuteResult{Operation: providerhost.ExecuteUp},
		&diagnostics,
		errors.New("update failed: the stack is currently locked by ci"),
	)
	if err != nil {
		t.Fatalf("err = %v, want the flag carried on a nil error", err)
	}
	if !result.ConcurrentUpdate {
		t.Fatalf("result = %+v, want the concurrent-update flag", result)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("diagnostics were dropped")
	}
}

func TestClassifyExecuteErrorLeavesGenericFailuresRaw(t *testing.T) {
	cause := errors.New("engine exploded")
	_, err := classifyExecuteError(providerhost.ExecuteResult{}, &bytes.Buffer{}, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the raw cause", err)
	}
}
