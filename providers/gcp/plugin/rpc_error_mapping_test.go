package plugin

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/magelift/magelift/sdk"
)

// TestSetErrorMapsEveryRegisteredRPCResponse instantiates each concrete
// response type of exported RPCServer methods and requires setError to
// store a sentinel. A new RPC method whose result is missing from the
// type switch fails here.
func TestSetErrorMapsEveryRegisteredRPCResponse(t *testing.T) {
	t.Parallel()
	serverType := reflect.TypeOf(&RPCServer{})
	if serverType.NumMethod() == 0 {
		t.Fatal("RPCServer has no exported methods")
	}
	for i := 0; i < serverType.NumMethod(); i++ {
		method := serverType.Method(i)
		if method.Type.NumIn() != 3 || method.Type.NumOut() != 1 {
			t.Fatalf("RPCServer.%s signature = %s, want (req, resp) error", method.Name, method.Type)
		}
		if method.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			t.Fatalf("RPCServer.%s result is %s, want error", method.Name, method.Type.Out(0))
		}
		respType := method.Type.In(2)
		if respType.Kind() != reflect.Ptr || respType.Elem().Kind() != reflect.Struct {
			t.Fatalf("RPCServer.%s response %s is not a struct pointer", method.Name, respType)
		}

		instance := reflect.New(respType.Elem())
		sentinel := &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "sentinel-" + method.Name}
		if !setError(instance.Interface(), sentinel) {
			t.Fatalf("setError did not map RPCServer.%s response %s", method.Name, respType)
		}
		errField := instance.Elem().FieldByName("Error")
		if !errField.IsValid() || errField.Kind() != reflect.Ptr || errField.IsNil() {
			t.Fatalf("RPCServer.%s Error field = %#v", method.Name, errField)
		}
		got, ok := errField.Interface().(*sdk.OperationError)
		if !ok || got != sentinel {
			t.Fatalf("RPCServer.%s stored %#v, want sentinel", method.Name, errField.Interface())
		}

		typedNil := reflect.Zero(respType).Interface()
		if setError(typedNil, sentinel) {
			t.Fatalf("setError accepted typed nil %s", respType)
		}
	}
}

func TestDispatchDeployAppPhaseFailureAndTimeout(t *testing.T) {
	t.Parallel()
	sentinel := &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "default website missing"}
	server := &RPCServer{Server: &Server{}}
	var failed sdk.DeployAppPhaseResult
	err := dispatch(server, sdk.OpDeployAppPhase, sdk.ProtocolV1, &failed, func(context.Context) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
		return nil, sentinel
	})
	if err != nil {
		t.Fatalf("operation failure returned transport error %v", err)
	}
	if failed.Error != sentinel {
		t.Fatalf("operation failure Error = %#v", failed.Error)
	}

	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	timing := &RPCServer{Server: &Server{Timeouts: map[sdk.Operation]Timeout{
		sdk.OpDeployAppPhase: {Duration: 20 * time.Millisecond, RetryableOnTimeout: false},
	}}}
	var timedOut sdk.DeployAppPhaseResult
	err = dispatch(timing, sdk.OpDeployAppPhase, sdk.ProtocolV1, &timedOut, func(context.Context) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
		<-hold
		return &sdk.DeployAppPhaseResult{Message: "late"}, nil
	})
	if err != nil {
		t.Fatalf("timeout returned transport error %v", err)
	}
	if timedOut.Error == nil || timedOut.Error.Code != sdk.ErrCodeTimeout || timedOut.Error.Retryable {
		t.Fatalf("timeout Error = %#v", timedOut.Error)
	}
	if timedOut.Message != "" {
		t.Fatalf("timeout wrote handler payload %q", timedOut.Message)
	}
}

func TestDispatchUnknownResponseFailsClosed(t *testing.T) {
	t.Parallel()
	type unmappedRPCResult struct {
		Error *sdk.OperationError
		Note  string
	}
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	server := &RPCServer{Server: &Server{Timeouts: map[sdk.Operation]Timeout{
		sdk.OpDescribe: {Duration: 20 * time.Millisecond, RetryableOnTimeout: true},
	}}}

	var failed unmappedRPCResult
	err := dispatch(server, sdk.OpDescribe, sdk.ProtocolV1, &failed, func(context.Context) (*unmappedRPCResult, *sdk.OperationError) {
		return nil, &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "secret-payload"}
	})
	if err == nil {
		t.Fatal("unmapped operation error was dropped")
	}
	if err != errCannotEncodeOperation {
		t.Fatalf("transport error = %v", err)
	}
	if failed.Error != nil || failed.Note != "" {
		t.Fatalf("unmapped response was written: %#v", failed)
	}

	var timedOut unmappedRPCResult
	err = dispatch(server, sdk.OpDescribe, sdk.ProtocolV1, &timedOut, func(context.Context) (*unmappedRPCResult, *sdk.OperationError) {
		<-hold
		return &unmappedRPCResult{Note: "late"}, nil
	})
	if err != errCannotEncodeOperation {
		t.Fatalf("unmapped timeout error = %v", err)
	}
	if timedOut.Error != nil || timedOut.Note != "" {
		t.Fatalf("unmapped timeout wrote response: %#v", timedOut)
	}
}
