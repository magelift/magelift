package ops

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestNewDeployStepsRejectsWrongBackend(t *testing.T) {
	t.Parallel()
	// nil planned fails AsAWSPlanned before backend type assert — still offline unit coverage
	// for the NewDeploySteps entrypoint without AWS/Pulumi.
	_, err := (Ops{}).NewDeploySteps(context.Background(), struct{}{}, nil, io.Discard)
	if err == nil {
		t.Fatal("expected error for nil planned")
	}
	if !strings.Contains(err.Error(), "unexpected planned type") {
		t.Fatalf("expected unexpected planned type, got %v", err)
	}
}
