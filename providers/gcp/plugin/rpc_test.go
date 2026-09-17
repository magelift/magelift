package plugin

import (
	"context"
	"net"
	"net/rpc"
	"strings"
	"testing"
	"time"

	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	"github.com/magelift/magelift/sdk"
)

func TestWireRoundTripOverNetRPC(t *testing.T) {
	t.Parallel()
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Plugin", &RPCServer{Server: &Server{Version: "wire-test"}}); err != nil {
		t.Fatal(err)
	}
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	go rpcServer.ServeConn(serverConn)
	client := rpc.NewClient(clientConn)
	defer client.Close()

	var described sdk.DescribeResponse
	if err := client.Call("Plugin.Describe", &sdk.DescribeRequest{ProtocolVersion: sdk.ProtocolV1}, &described); err != nil {
		t.Fatal(err)
	}
	if described.ProviderVersion != "wire-test" || len(described.Operations) != 31 || described.Error != nil {
		t.Fatalf("describe = %#v", described)
	}
	method, err := sdk.PluginMethod(sdk.OpDescribe)
	if err != nil {
		t.Fatal(err)
	}
	var describedAgain sdk.DescribeResponse
	if err := client.Call(method, &sdk.DescribeRequest{ProtocolVersion: sdk.ProtocolV1}, &describedAgain); err != nil {
		t.Fatal(err)
	}
	if describedAgain.ProviderID != "gcp" {
		t.Fatalf("describe via table = %#v", describedAgain)
	}
}

func TestDispatchRejectsBadVersion(t *testing.T) {
	t.Parallel()
	rpc := &RPCServer{Server: &Server{}}
	var resp sdk.PlanResult
	if err := rpc.Plan(&sdk.PlanRequest{ProtocolVersion: "2.0"}, &resp); err == nil {
		t.Fatal("incompatible version was accepted")
	}
	var resp2 sdk.PlanResult
	if err := rpc.Plan(&sdk.PlanRequest{}, &resp2); err == nil {
		t.Fatal("missing version was accepted")
	}
}

func TestDispatchTimeoutIsTyped(t *testing.T) {
	t.Parallel()
	blocking := &Server{
		NewStack: func(ctx context.Context, _ string, _ gcpstack.Spec, _ string) (Automation, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		Timeouts: map[sdk.Operation]Timeout{
			sdk.OpApply:   {Duration: time.Nanosecond},
			sdk.OpOutputs: {Duration: time.Nanosecond, RetryableOnTimeout: true},
		},
	}
	rpc := &RPCServer{Server: blocking}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	var applyResp sdk.LifecycleResult
	if err := rpc.Apply(&sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan}, &applyResp); err != nil {
		t.Fatalf("rpc error = %v", err)
	}
	if applyResp.Error == nil || applyResp.Error.Code != sdk.ErrCodeTimeout || applyResp.Error.Retryable {
		t.Fatalf("apply timeout = %v", applyResp.Error)
	}
	var outputsResp sdk.OutputsResult
	if err := rpc.Outputs(&sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan}, &outputsResp); err != nil {
		t.Fatalf("rpc error = %v", err)
	}
	if outputsResp.Error == nil || outputsResp.Error.Code != sdk.ErrCodeTimeout || !outputsResp.Error.Retryable {
		t.Fatalf("outputs timeout = %v", outputsResp.Error)
	}
}

func TestDispatchSurfacesHandlerErrors(t *testing.T) {
	t.Parallel()
	rpc := &RPCServer{Server: &Server{Admission: stubAdmission{}}}
	var resp sdk.PlanResult
	if err := rpc.Plan(&sdk.PlanRequest{ProtocolVersion: sdk.ProtocolV1}, &resp); err != nil {
		t.Fatalf("rpc error = %v", err)
	}
	if resp.Error == nil || resp.Error.Code != sdk.ErrCodeInvalid {
		t.Fatalf("handler error = %v", resp.Error)
	}
}

func TestRPCMethodsMatchPluginTable(t *testing.T) {
	t.Parallel()
	rpc := &RPCServer{Server: &Server{Version: "test"}}
	cases := map[sdk.Operation]func() error{
		sdk.OpDescribe: func() error {
			return rpc.Describe(&sdk.DescribeRequest{ProtocolVersion: sdk.ProtocolV1}, &sdk.DescribeResponse{})
		},
		sdk.OpValidateConfig: func() error {
			return rpc.ValidateConfig(&sdk.ValidateConfigRequest{ProtocolVersion: sdk.ProtocolV1}, &sdk.ValidateConfigResult{})
		},
	}
	for operation, call := range cases {
		method, err := sdk.PluginMethod(operation)
		if err != nil {
			t.Fatalf("operation %q has no method: %v", operation, err)
		}
		if !strings.HasPrefix(method, "Plugin.") {
			t.Fatalf("method %q is malformed", method)
		}
		if err := call(); err != nil {
			t.Fatalf("operation %q rpc error = %v", operation, err)
		}
	}
}
