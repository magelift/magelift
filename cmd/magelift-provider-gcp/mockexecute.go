package main

import (
	"fmt"

	"github.com/magelift/magelift/internal/providerhost"
)

// mockExecute answers Execute requests without touching Pulumi or any cloud.
// It proves the subprocess execution path (decode, dispatch, result mapping,
// error propagation) for Dial tests. Real graph coverage lives in the
// in-process Pulumi mock suites, and real execution is proven by live runs;
// the mock deliberately returns synthetic answers instead of re-running the
// graph through a second, lossy mock layer.
func mockExecute(request providerhost.ExecuteRequest) providerhost.ExecuteResult {
	result := providerhost.ExecuteResult{
		Operation: request.Operation,
		Diagnostics: []string{
			fmt.Sprintf("mock execute: %s %s (no provider mutation)", request.Operation, request.Plan.StackName),
		},
	}
	switch request.Operation {
	case providerhost.ExecutePreview, providerhost.ExecuteUp:
		result.Changes = map[string]int{"create": 1}
	case providerhost.ExecuteDestroy:
		result.Changes = map[string]int{"delete": 1}
	case providerhost.ExecuteOutputs:
		result.Outputs = map[string]any{"mock": true}
	case providerhost.ExecuteValidateRequest:
		// Valid: mock mode carries no persisted ownership record.
	}
	return result
}
