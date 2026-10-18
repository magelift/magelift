package plugin

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	"github.com/magelift/magelift/sdk"
)

func TestApplySQLStockoutReportsScopeAndSafeNextAction(t *testing.T) {
	message := "Error waiting for Create Instance: The zone or region does not have sufficient resources to handle the request at the moment."
	backend := &stubAutomation{err: errors.New(message)}
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) { return backend, nil }}
	_, failure := server.Apply(context.Background(), &sdk.StackCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), Plan: storedTestPlan(t, testSpec()),
	})
	if failure == nil || failure.Code != sdk.ErrCodeUpstream || failure.Retryable {
		t.Fatalf("failure = %#v", failure)
	}
	for _, want := range []string{"project=shop", "gcpProject=example-gcp-project", "environment=staging", "stage=infra-update", "outcome=failed", "state retained", message, "target.gcp.zones", "same digest", "Do not remove the state"} {
		if !strings.Contains(failure.Message, want) {
			t.Errorf("missing diagnostic %q", want)
		}
	}
	if !slices.Equal(backend.calls, []string{"update"}) {
		t.Fatalf("unexpected retry or mutation: %v", backend.calls)
	}
}

func TestCapacityGuidanceDoesNotChangeOtherErrorsOrDestroy(t *testing.T) {
	for _, destroying := range []bool{false, true} {
		message := "unrelated upstream failure"
		if destroying {
			message = "Error waiting for Create Instance: ERROR_STOCKOUT"
		}
		backend := &stubAutomation{err: errors.New(message)}
		server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) { return backend, nil }}
		call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), Plan: storedTestPlan(t, testSpec())}
		var failure *sdk.OperationError
		if destroying {
			_, failure = server.Destroy(context.Background(), call)
		} else {
			_, failure = server.Apply(context.Background(), call)
		}
		if failure == nil || failure.Message != message || failure.Retryable {
			t.Fatalf("destroy=%v failure=%#v", destroying, failure)
		}
	}
}
