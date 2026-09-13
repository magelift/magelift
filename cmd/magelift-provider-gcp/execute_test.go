package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/magelift/magelift/internal/providerhost"
)

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
